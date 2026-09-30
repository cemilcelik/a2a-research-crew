package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2a"
)

// renderEvent, akıştan gelen bir A2A olayını okunabilir biçimde yazar ve
// artefakt metnini toplar.
func renderEvent(w io.Writer, event a2a.Event, collected *strings.Builder) {
	switch e := event.(type) {
	case *a2a.Task:
		fmt.Fprintf(w, "task %s state=%s\n", e.ID, e.Status.State)
	case *a2a.TaskStatusUpdateEvent:
		if text := partsText(e.Status.Message); text != "" {
			fmt.Fprintf(w, "[%s] %s\n", e.Status.State, text)
			return
		}
		fmt.Fprintf(w, "[%s]\n", e.Status.State)
	case *a2a.TaskArtifactUpdateEvent:
		text := messagePartsText(e.Artifact.Parts)
		collected.WriteString(text)
		fmt.Fprintf(w, "artifact %q updated (%d chars)\n", e.Artifact.Name, len(text))
	case *a2a.Message:
		fmt.Fprintf(w, "%s\n", partsText(e))
	}
}

// partsText, bir mesajdaki metin parçalarını birleştirir.
func partsText(message *a2a.Message) string {
	if message == nil {
		return ""
	}
	var b strings.Builder
	for _, part := range message.Parts {
		if text := part.Text(); text != "" {
			b.WriteString(text)
		}
	}
	return b.String()
}

// artifactText, bir görevin artefaktlarını birleştirir.
func artifactText(task *a2a.Task) string {
	var b strings.Builder
	for _, artifact := range task.Artifacts {
		if text := messagePartsText(artifact.Parts); text != "" {
			b.WriteString(text)
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func messagePartsText(parts a2a.ContentParts) string {
	var b strings.Builder
	for _, part := range parts {
		if text := part.Text(); text != "" {
			b.WriteString(text)
		}
	}
	return b.String()
}
