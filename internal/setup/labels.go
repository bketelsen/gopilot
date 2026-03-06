package setup

import "github.com/bketelsen/gopilot/internal/config"

// LabelDef defines a label with its canonical color and description.
type LabelDef struct {
	Name        string
	Color       string
	Description string
}

// RequiredLabels returns the set of labels that gopilot setup should ensure
// exist on each configured repository.
func RequiredLabels(cfg *config.Config) []LabelDef {
	var labels []LabelDef

	// Eligible labels from config (each gets the blue color)
	for _, name := range cfg.GitHub.EligibleLabels {
		labels = append(labels, LabelDef{
			Name:        name,
			Color:       "0052CC",
			Description: "Eligible for Gopilot agent dispatch",
		})
	}

	// Planning label
	labels = append(labels, LabelDef{
		Name:        cfg.Planning.Label,
		Color:       "7B61FF",
		Description: "Gopilot interactive planning",
	})

	// Completed planning label
	labels = append(labels, LabelDef{
		Name:        cfg.Planning.CompletedLabel,
		Color:       "1D7644",
		Description: "Planning completed by Gopilot",
	})

	// Failure label (hard-coded in orchestrator)
	labels = append(labels, LabelDef{
		Name:        "gopilot-failed",
		Color:       "D93F0B",
		Description: "Gopilot agent failed after max retries",
	})

	return labels
}
