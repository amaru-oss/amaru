package httpapi

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rluders/amaru/services/platform/internal/adapter/localauth"
	"github.com/rluders/amaru/services/platform/internal/coordinator"
	"github.com/rluders/amaru/services/platform/internal/domain"
	"github.com/rluders/amaru/services/platform/internal/ports"
)

// executionEventPatterns are the served Execution Event operations: listExecutionEvents.
var executionEventPatterns = []string{
	"GET /v1/projects/{projectId}/executions/{id}/events",
}

// maxEventPage is the replay core's page bound and the contract's `limit` maximum and default.
const maxEventPage = 1000

var cursorRe = regexp.MustCompile(`^[0-9]{1,18}$`)

// Journal kinds that have no faithful public event shape. They are omitted from a page; the
// cursor still advances past them. Approval decisions are omitted because the journal entry
// lacks the Approval id and evidence digest ApprovalEvent requires; the stale-result entry lacks
// the fencing tokens ResultRejectedStale requires.
var omittedEventKinds = map[string]bool{
	domain.EventStepRetried:         true,
	domain.EventStepResumed:         true,
	domain.EventStepDeferred:        true,
	domain.EventStepRejected:        true,
	domain.EventStepRemoteJob:       true,
	domain.EventExecCancelRequested: true,
	domain.EventResultStale:         true,
	domain.EventApprovalApproved:    true,
	domain.EventApprovalRejected:    true,
}

// Execution lifecycle kinds keep their name as the public type and carry the state they enter.
var lifecycleEventStates = map[string]domain.ExecState{
	domain.EventExecWaiting:   domain.ExecWaiting,
	domain.EventExecCompleted: domain.ExecSucceeded,
	domain.EventExecCancelled: domain.ExecCancelled,
}

// Step kinds become `execution.step` with the state the Step entered.
var stepEventStates = map[string]domain.StepState{
	domain.EventStepLeased:      domain.StepLeased,
	domain.EventStepStarted:     domain.StepRunning,
	domain.EventStepSucceeded:   domain.StepSucceeded,
	domain.EventStepFailed:      domain.StepFailed,
	domain.EventStepTimedOut:    domain.StepTimedOut,
	domain.EventStepInterrupted: domain.StepInterrupted,
	domain.EventStepCancelled:   domain.StepCancelled,
}

// ExecutionEventsHandler includes the compact Host/Origin/session/CSRF guard. access must use
// this Auth instance and the same authoritative Instance stores as stream; window is the
// trusted deployment replay window, never client input.
func ExecutionEventsHandler(auth *localauth.Auth, access *coordinator.ProjectAccess, stream ports.EventStream, window time.Duration) (http.Handler, error) {
	if auth == nil || access == nil || stream == nil || window <= 0 {
		return nil, domain.ErrInvalid
	}
	mux := http.NewServeMux()
	mux.HandleFunc(executionEventPatterns[0], func(w http.ResponseWriter, r *http.Request) {
		project, id := domain.ProjectKey(r.PathValue("projectId")), r.PathValue("id")
		q, ok := eventQuery(r.URL.Query())
		if !ok {
			reject(w, 400, "invalid_request")
			return
		}
		q.ProjectID, q.ExecutionID = string(project), id
		// ProjectAccess authorizes the Project and the Execution's ownership before the read.
		page, err := access.Events(r.Context(), q, stream, window)
		if err != nil {
			accessError(w, err)
			return
		}
		if page.Resync != nil {
			resync, err := wireResync(*page.Resync)
			if err != nil {
				accessError(w, err)
				return
			}
			respond(w, http.StatusOK, wireEventList{Events: []wireEvent{}, Resync: resync})
			return
		}
		// Read after the page: a failure the page reports is terminal, so the record read now
		// carries the error recorded with it.
		e, steps, err := access.Execution(r.Context(), project, id)
		if err != nil {
			accessError(w, err)
			return
		}
		out := wireEventList{Events: make([]wireEvent, 0, len(page.Events)), NextCursor: &page.Cursor}
		for _, se := range page.Events {
			ev, ok, err := wireStreamEvent(se, e, steps)
			if err != nil {
				// One unrepresentable entry fails the page; none is silently dropped.
				accessError(w, err)
				return
			}
			if ok {
				out.Events = append(out.Events, ev)
			}
		}
		respond(w, http.StatusOK, out)
	})
	return auth.Handler(mux), nil
}

// eventQuery accepts only the contract's `limit` (1..1000, default 1000) and `after` (>= 0,
// default 0), each at most once, as plain decimal digits.
func eventQuery(values url.Values) (domain.EventQuery, bool) {
	q := domain.EventQuery{Limit: maxEventPage}
	for name, vals := range values {
		if len(vals) != 1 || !cursorRe.MatchString(vals[0]) {
			return q, false
		}
		n, err := strconv.ParseInt(vals[0], 10, 64)
		if err != nil {
			return q, false
		}
		switch name {
		case "limit":
			if n < 1 || n > maxEventPage {
				return q, false
			}
			q.Limit = int(n)
		case "after":
			q.After = n
		default:
			return q, false
		}
	}
	return q, true
}

type wireResource struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type wireEvent struct {
	ID          string       `json:"id"`
	Seq         int64        `json:"seq"`
	Type        string       `json:"type"`
	Time        string       `json:"time"`
	ProjectID   string       `json:"projectId"`
	ExecutionID string       `json:"executionId"`
	Resource    wireResource `json:"resource"`
	Data        any          `json:"data"`
}

type wireLifecycleData struct {
	State string `json:"state"`
}

type wireFailedData struct {
	State string         `json:"state"`
	Error *wireExecError `json:"error"`
}

type wireStepEventData struct {
	StepID string         `json:"stepId"`
	State  string         `json:"state"`
	Error  *wireExecError `json:"error,omitempty"`
}

type wireEventList struct {
	Events     []wireEvent            `json:"events"`
	NextCursor *int64                 `json:"nextCursor,omitempty"`
	Resync     *domain.ResyncRequired `json:"resync,omitempty"`
}

func wireResync(r domain.ResyncRequired) (*domain.ResyncRequired, error) {
	switch r.Reason {
	case "cursor_too_old", "cursor_ahead", "history_unavailable":
	default:
		return nil, fmt.Errorf("%w: resync reason", errUnrepresentable)
	}
	if r.Type != "resync_required" || r.ResumeFrom < 0 {
		return nil, fmt.Errorf("%w: resync", errUnrepresentable)
	}
	return &r, nil
}

// wireStreamEvent maps one journal entry of Execution e. ok is false for an omitted kind.
// Failure errors come from the current record: failed Steps and failed Executions are terminal,
// so the recorded error is the one written with the event. An unknown kind is refused so a new
// journal kind gets an explicit mapping decision.
func wireStreamEvent(se domain.StreamEvent, e domain.Execution, steps []domain.StepExecution) (wireEvent, bool, error) {
	ev := se.Event
	if se.ProjectID != e.ProjectID || ev.ExecutionID != e.ID || se.Seq < 1 {
		return wireEvent{}, false, fmt.Errorf("%w: event outside the query", errUnrepresentable)
	}
	if omittedEventKinds[ev.Kind] {
		return wireEvent{}, false, nil
	}
	id, err := eventID(se.ProjectID, se.Seq, ev.At)
	if err != nil {
		return wireEvent{}, false, err
	}
	out := wireEvent{
		ID: id, Seq: se.Seq, Type: ev.Kind, Time: timestamp(ev.At), ProjectID: se.ProjectID,
		ExecutionID: e.ID, Resource: wireResource{Type: "execution", ID: e.ID},
	}
	if state, ok := lifecycleEventStates[ev.Kind]; ok {
		out.Data = wireLifecycleData{State: string(state)}
		return out, true, nil
	}
	if ev.Kind == domain.EventExecFailed {
		if e.State != domain.ExecFailed {
			return wireEvent{}, false, fmt.Errorf("%w: failure event of a %s execution", errUnrepresentable, e.State)
		}
		execErr, err := wireError(e.Error, true)
		if err != nil {
			return wireEvent{}, false, err
		}
		out.Data = wireFailedData{State: string(domain.ExecFailed), Error: execErr}
		return out, true, nil
	}
	state, ok := stepEventStates[ev.Kind]
	if !ok {
		return wireEvent{}, false, fmt.Errorf("%w: event kind %q", errUnrepresentable, ev.Kind)
	}
	stepID, ok := strings.CutPrefix(ev.StepID, e.ID+".")
	if !ok || stepID == "" {
		return wireEvent{}, false, fmt.Errorf("%w: event step identity", errUnrepresentable)
	}
	data := wireStepEventData{StepID: stepID, State: string(state)}
	if state == domain.StepFailed {
		var failed *domain.StepExecution
		for i := range steps {
			if steps[i].ID == ev.StepID {
				failed = &steps[i]
			}
		}
		if failed == nil || failed.State != domain.StepFailed {
			return wireEvent{}, false, fmt.Errorf("%w: failure event of a step not failed", errUnrepresentable)
		}
		if data.Error, err = wireError(failed.Error, true); err != nil {
			return wireEvent{}, false, err
		}
	}
	out.Type, out.Data = "execution.step", data
	return out, true, nil
}

// eventID derives the public event id: the event time's Unix milliseconds, then the first 80
// bits of SHA-256 over the Project id and cursor, in ULID text form. The journal stores no
// event id; this one is the same on every read and unique per Project cursor, but carries no
// randomness.
func eventID(projectID string, seq int64, at time.Time) (string, error) {
	ms := at.UnixMilli()
	if at.IsZero() || ms < 0 || ms >= 1<<48 {
		return "", fmt.Errorf("%w: event time", errUnrepresentable)
	}
	sum := sha256.Sum256([]byte("amaru.event.v1\x00" + projectID + "\x00" + strconv.FormatInt(seq, 10)))
	var b [16]byte
	for i := 5; i >= 0; i-- {
		b[i] = byte(ms)
		ms >>= 8
	}
	copy(b[6:], sum[:10])
	const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	hi := uint64(b[0])<<56 | uint64(b[1])<<48 | uint64(b[2])<<40 | uint64(b[3])<<32 | uint64(b[4])<<24 | uint64(b[5])<<16 | uint64(b[6])<<8 | uint64(b[7])
	lo := uint64(b[8])<<56 | uint64(b[9])<<48 | uint64(b[10])<<40 | uint64(b[11])<<32 | uint64(b[12])<<24 | uint64(b[13])<<16 | uint64(b[14])<<8 | uint64(b[15])
	var out [26]byte
	// 128 bits as 26 base32 digits: the first digit carries the top 3 bits.
	for i := 25; i >= 0; i-- {
		out[i] = crockford[lo&31]
		lo = lo>>5 | hi<<59
		hi >>= 5
	}
	return string(out[:]), nil
}
