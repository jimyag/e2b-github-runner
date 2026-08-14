package runnercatalog

import "sort"

// PublicTemplate is the stable, runnerd-owned metadata for one managed public
// Sandbox template. It intentionally excludes provider and scoped metadata.
type PublicTemplate struct {
	DefaultTemplateName string     `json:"default_template_name"`
	RunnerSpecNames     []string   `json:"runner_spec_names"`
	WorkflowLabels      [][]string `json:"workflow_labels"`
}

// PublicTemplates derives public template metadata from the managed catalog.
func PublicTemplates() []PublicTemplate {
	byTemplateName := make(map[string]*PublicTemplate)
	for _, profile := range DefaultProfiles() {
		template := byTemplateName[profile.DefaultTemplateName]
		if template == nil {
			template = &PublicTemplate{DefaultTemplateName: profile.DefaultTemplateName}
			byTemplateName[profile.DefaultTemplateName] = template
		}
		template.RunnerSpecNames = append(template.RunnerSpecNames, profile.Name)
		template.WorkflowLabels = append(template.WorkflowLabels, append([]string(nil), profile.RequiredLabels...))
	}

	templates := make([]PublicTemplate, 0, len(byTemplateName))
	for _, template := range byTemplateName {
		sort.Strings(template.RunnerSpecNames)
		sort.Slice(template.WorkflowLabels, func(i, j int) bool {
			return labelsLess(template.WorkflowLabels[i], template.WorkflowLabels[j])
		})
		templates = append(templates, *template)
	}
	sort.Slice(templates, func(i, j int) bool {
		return templates[i].DefaultTemplateName < templates[j].DefaultTemplateName
	})
	return templates
}

func labelsLess(left, right []string) bool {
	for index := 0; index < len(left) && index < len(right); index++ {
		if left[index] != right[index] {
			return left[index] < right[index]
		}
	}
	return len(left) < len(right)
}
