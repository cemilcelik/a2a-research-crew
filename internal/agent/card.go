// Package agent, ajanların A2A AgentCard bildirimlerini ve paylaşılan ajan
// kimliği kavramlarını üretir.
package agent

import "github.com/a2aproject/a2a-go/v2/a2a"

// BearerSchemeName, HTTP Bearer (JWT) güvenlik şemasının AgentCard anahtarıdır.
const BearerSchemeName a2a.SecuritySchemeName = "bearer"

// DefaultInputModes, bir ajan için varsayılan girdi MIME tipleridir.
var DefaultInputModes = []string{"text/plain", "application/json"}

// DefaultOutputModes, bir ajan için varsayılan çıktı MIME tipleridir.
var DefaultOutputModes = []string{"text/plain", "application/json"}

// Interface, bir ajanın sunduğu tek bir taşıma arayüzüdür.
type Interface struct {
	URL      string
	Protocol a2a.TransportProtocol
}

// Skill, bir ajanın AgentCard'ında bildirdiği tek bir yetenektir.
type Skill struct {
	ID          string
	Name        string
	Description string
	Tags        []string
	Examples    []string
}

// CardSpec, bir AgentCard üretmek için gereken bildirim bilgisidir.
type CardSpec struct {
	Name        string
	Description string
	Version     string
	Provider    *a2a.AgentProvider
	Interfaces  []Interface
	Skills      []Skill

	Streaming         bool
	PushNotifications bool

	InputModes  []string
	OutputModes []string

	// RequireBearer, kartın HTTP Bearer (JWT) güvenlik şemasını gerektirdiğini
	// bildirir.
	RequireBearer bool
}

// BuildCard, verilen spesifikasyondan bir *a2a.AgentCard üretir.
func BuildCard(spec CardSpec) *a2a.AgentCard {
	card := &a2a.AgentCard{
		Name:                spec.Name,
		Description:         spec.Description,
		Version:             spec.Version,
		Provider:            spec.Provider,
		Capabilities:        a2a.AgentCapabilities{Streaming: spec.Streaming, PushNotifications: spec.PushNotifications},
		DefaultInputModes:   orDefault(spec.InputModes, DefaultInputModes),
		DefaultOutputModes:  orDefault(spec.OutputModes, DefaultOutputModes),
		Skills:              buildSkills(spec.Skills),
		SupportedInterfaces: buildInterfaces(spec.Interfaces),
	}
	if spec.RequireBearer {
		card.SecuritySchemes = a2a.NamedSecuritySchemes{
			BearerSchemeName: a2a.HTTPAuthSecurityScheme{
				Scheme:       "bearer",
				BearerFormat: "JWT",
				Description:  "JWT access token issued by the orchestrator.",
			},
		}
		card.SecurityRequirements = a2a.SecurityRequirementsOptions{
			a2a.SecurityRequirements{BearerSchemeName: a2a.SecuritySchemeScopes{}},
		}
	}
	return card
}

func buildInterfaces(interfaces []Interface) []*a2a.AgentInterface {
	out := make([]*a2a.AgentInterface, 0, len(interfaces))
	for _, iface := range interfaces {
		out = append(out, a2a.NewAgentInterface(iface.URL, iface.Protocol))
	}
	return out
}

func buildSkills(skills []Skill) []a2a.AgentSkill {
	out := make([]a2a.AgentSkill, 0, len(skills))
	for _, s := range skills {
		out = append(out, a2a.AgentSkill{
			ID:          s.ID,
			Name:        s.Name,
			Description: s.Description,
			Tags:        s.Tags,
			Examples:    s.Examples,
		})
	}
	return out
}

func orDefault(value, fallback []string) []string {
	if len(value) == 0 {
		return fallback
	}
	return value
}
