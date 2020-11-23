package common

type Conf struct {
	Provider          string `env:"provider"`
	ProjectPath       string `env:"project_path"`
	RepoOwner         string `env:"repo_owner"`
	RepoName          string `env:"repo_name"`
	AuthToken         string `env:"auth_token,required"`
	PullRequest       int    `env:"pull_request"`
	CommitHash        string `env:"commit_hash"`
	VariantLabels     string `env:"variant_labels,required"`
	VariantPatterns   string `env:"variant_patterns,required"`
	ExportDescription string `env:"export_description"`
	Labels2Env        string `env:"labels2env"`
}
