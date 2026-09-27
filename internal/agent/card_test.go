package agent

import (
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"
)

func TestBuildCard(t *testing.T) {
	card := BuildCard(CardSpec{
		Name:        "market-scout",
		Description: "Gathers market data",
		Version:     "0.1.0",
		Interfaces: []Interface{
			{URL: "grpc://localhost:9101", Protocol: a2a.TransportProtocolGRPC},
		},
		Skills: []Skill{
			{ID: "market_research", Name: "Market research", Tags: []string{"research"}},
		},
		Streaming:     true,
		RequireBearer: true,
	})

	if card.Name != "market-scout" {
		t.Errorf("Name = %q, want market-scout", card.Name)
	}
	if len(card.SupportedInterfaces) != 1 || card.SupportedInterfaces[0].ProtocolBinding != a2a.TransportProtocolGRPC {
		t.Fatalf("SupportedInterfaces = %+v, want one gRPC interface", card.SupportedInterfaces)
	}
	if len(card.Skills) != 1 || card.Skills[0].ID != "market_research" {
		t.Fatalf("Skills = %+v, want market_research", card.Skills)
	}
	if !card.Capabilities.Streaming {
		t.Error("Capabilities.Streaming = false, want true")
	}
	if len(card.DefaultInputModes) == 0 || len(card.DefaultOutputModes) == 0 {
		t.Error("default IO modes must be populated")
	}
	if _, ok := card.SecuritySchemes[BearerSchemeName]; !ok {
		t.Errorf("SecuritySchemes = %+v, want %q", card.SecuritySchemes, BearerSchemeName)
	}
	if len(card.SecurityRequirements) != 1 {
		t.Errorf("SecurityRequirements = %+v, want one requirement", card.SecurityRequirements)
	}
}

func TestBuildCardNoSecurity(t *testing.T) {
	card := BuildCard(CardSpec{Name: "no-auth", Version: "0.1.0"})
	if card.SecuritySchemes != nil {
		t.Errorf("SecuritySchemes = %+v, want nil", card.SecuritySchemes)
	}
	if len(card.SecurityRequirements) != 0 {
		t.Errorf("SecurityRequirements = %+v, want empty", card.SecurityRequirements)
	}
}
