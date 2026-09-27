package taskstore

import (
	"fmt"

	"github.com/a2aproject/a2a-go/v2/a2a"
)

// validateTask, görev içindeki metadata'nın A2A spesifikasyonunun izin verdiği
// tiplerden oluştuğunu doğrular. SDK'nın in-memory doğrulamasıyla aynı kuralları
// uygular.
func validateTask(task *a2a.Task) error {
	if task == nil {
		return nil
	}
	if err := validateMessage(task.Status.Message); err != nil {
		return err
	}
	for _, msg := range task.History {
		if err := validateMessage(msg); err != nil {
			return err
		}
	}
	for _, artifact := range task.Artifacts {
		if err := validateArtifact(artifact); err != nil {
			return err
		}
	}
	return validateMeta(task.Metadata)
}

func validateArtifact(artifact *a2a.Artifact) error {
	if artifact == nil {
		return nil
	}
	if err := validateParts(artifact.Parts); err != nil {
		return err
	}
	return validateMeta(artifact.Metadata)
}

func validateMessage(msg *a2a.Message) error {
	if msg == nil {
		return nil
	}
	if err := validateParts(msg.Parts); err != nil {
		return err
	}
	return validateMeta(msg.Metadata)
}

func validateParts(parts a2a.ContentParts) error {
	for _, p := range parts {
		if err := validateMeta(p.Meta()); err != nil {
			return err
		}
	}
	return nil
}

func validateMeta(meta map[string]any) error {
	return validateMetaRecursive(meta, map[string]struct{}{})
}

func validateMetaRecursive(value any, processing map[string]struct{}) error {
	if value == nil {
		return nil
	}
	switch value.(type) {
	// uint tipleri spesifikasyonla uyumsuz olduğu için hariç tutulur.
	case bool, int, int8, int16, int32, int64, float32, float64, string:
		return nil
	}

	key := fmt.Sprintf("%p", value)
	if _, ok := processing[key]; ok {
		return fmt.Errorf("circular reference in Metadata")
	}
	processing[key] = struct{}{}
	defer delete(processing, key)

	if arr, ok := value.([]any); ok {
		for _, elem := range arr {
			if err := validateMetaRecursive(elem, processing); err != nil {
				return err
			}
		}
		return nil
	}
	if m, ok := value.(map[string]any); ok {
		for _, elem := range m {
			if err := validateMetaRecursive(elem, processing); err != nil {
				return err
			}
		}
		return nil
	}
	return fmt.Errorf("%T is not permitted in Metadata, must be one of nil, bool, int, float, string, []any, map[string]any", value)
}
