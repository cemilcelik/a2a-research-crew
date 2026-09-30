package cli

import (
	"strings"
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"
)

func TestRenderEventsCollectsArtifact(t *testing.T) {
	var out strings.Builder
	var collected strings.Builder

	task := &a2a.Task{ID: "t1", ContextID: "c1", Status: a2a.TaskStatus{State: a2a.TaskStateSubmitted}}
	renderEvent(&out, task, &collected)

	working := a2a.NewStatusUpdateEvent(taskInfo("t1", "c1"), a2a.TaskStateWorking, nil)
	renderEvent(&out, working, &collected)

	artifact := a2a.NewArtifactEvent(taskInfo("t1", "c1"), a2a.NewTextPart("research report body"))
	renderEvent(&out, artifact, &collected)

	completed := a2a.NewStatusUpdateEvent(taskInfo("t1", "c1"), a2a.TaskStateCompleted, nil)
	renderEvent(&out, completed, &collected)

	if !strings.Contains(out.String(), "TASK_STATE_WORKING") || !strings.Contains(out.String(), "TASK_STATE_COMPLETED") {
		t.Fatalf("rendered output = %q, want status lines", out.String())
	}
	if collected.String() != "research report body" {
		t.Fatalf("collected = %q, want artifact text", collected.String())
	}
}

func taskInfo(taskID, contextID string) a2a.TaskInfoProvider {
	return a2a.TaskInfo{TaskID: a2a.TaskID(taskID), ContextID: contextID}
}
