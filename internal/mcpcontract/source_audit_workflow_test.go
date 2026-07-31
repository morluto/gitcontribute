package mcpcontract

import "testing"

func TestCanonicalSourceAuditWorkflowTransitionsAndAuthorities(t *testing.T) {
	wf := CanonicalSourceAuditWorkflow()
	if wf.Version != SourceAuditWorkflowVersion || len(wf.Transitions) != 9 {
		t.Fatalf("workflow = %+v", wf)
	}
	for i, transition := range wf.Transitions {
		if transition.ID == "" || transition.Operation == "" || transition.ExpectedResultType == "" || transition.IncompleteSemantics == "" {
			t.Fatalf("transition %d is incomplete: %+v", i, transition)
		}
	}
	if wf.Transitions[0].Authority.Network || !wf.Transitions[1].Authority.Network || !wf.Transitions[1].Authority.LocalWrite {
		t.Fatalf("coverage authorities = %+v -> %+v", wf.Transitions[0].Authority, wf.Transitions[1].Authority)
	}
	if wf.Transitions[3].RequiredInputToken != "snapshot_token" || wf.Transitions[3].Authority.Network {
		t.Fatalf("offline reread transition = %+v", wf.Transitions[3])
	}
}
