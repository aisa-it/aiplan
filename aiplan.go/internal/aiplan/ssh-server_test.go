package aiplan

import (
	"strings"
	"testing"
)

// TestParseGitCommand проверяет парсинг Git команд
func TestParseGitCommand(t *testing.T) {
	tests := []struct {
		name        string
		command     []string
		wantGitCmd  string
		wantPath    string
		wantErr     bool
		errContains string
	}{
		{
			name:       "Valid git-upload-pack",
			command:    []string{"git-upload-pack", "/workspace/repo.git"},
			wantGitCmd: "git-upload-pack",
			wantPath:   "/workspace/repo.git",
			wantErr:    false,
		},
		{
			name:       "Valid git-receive-pack",
			command:    []string{"git-receive-pack", "workspace/repo.git"},
			wantGitCmd: "git-receive-pack",
			wantPath:   "workspace/repo.git",
			wantErr:    false,
		},
		{
			name:       "Valid git-upload-archive",
			command:    []string{"git-upload-archive", "'workspace/repo.git'"},
			wantGitCmd: "git-upload-archive",
			wantPath:   "workspace/repo.git",
			wantErr:    false,
		},
		{
			name:        "Invalid command - too few arguments",
			command:     []string{"git-upload-pack"},
			wantErr:     true,
			errContains: "invalid command format",
		},
		{
			name:        "Invalid command - unsupported git command",
			command:     []string{"git-shell", "/workspace/repo.git"},
			wantErr:     true,
			errContains: "unsupported git command",
		},
		{
			name:       "Path with quotes",
			command:    []string{"git-upload-pack", "\"workspace/repo.git\""},
			wantGitCmd: "git-upload-pack",
			wantPath:   "workspace/repo.git",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotGitCmd, gotPath, err := parseGitCommand(tt.command)

			if tt.wantErr {
				if err == nil {
					t.Errorf("parseGitCommand() expected error, got nil")
					return
				}
				if tt.errContains != "" && err != nil {
					if !strings.Contains(err.Error(), tt.errContains) {
						t.Errorf("parseGitCommand() error = %v, want error containing %q", err, tt.errContains)
					}
				}
				return
			}

			if err != nil {
				t.Errorf("parseGitCommand() unexpected error = %v", err)
				return
			}

			if gotGitCmd != tt.wantGitCmd {
				t.Errorf("parseGitCommand() gitCmd = %v, want %v", gotGitCmd, tt.wantGitCmd)
			}

			if gotPath != tt.wantPath {
				t.Errorf("parseGitCommand() path = %v, want %v", gotPath, tt.wantPath)
			}
		})
	}
}

// TestParseRepositoryPath проверяет парсинг пути репозитория
func TestParseRepositoryPath(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		wantWorkspace string
		wantRepo      string
		wantErr       bool
		errContains   string
	}{
		{
			name:          "Valid path with .git",
			path:          "workspace/repo.git",
			wantWorkspace: "workspace",
			wantRepo:      "repo",
			wantErr:       false,
		},
		{
			name:          "Valid path without .git",
			path:          "workspace/repo",
			wantWorkspace: "workspace",
			wantRepo:      "repo",
			wantErr:       false,
		},
		{
			name:          "Valid path with leading slash",
			path:          "/workspace/repo.git",
			wantWorkspace: "workspace",
			wantRepo:      "repo",
			wantErr:       false,
		},
		{
			name:          "Valid path with dashes and underscores",
			path:          "my-workspace/my_repo-name.git",
			wantWorkspace: "my-workspace",
			wantRepo:      "my_repo-name",
			wantErr:       false,
		},
		{
			name:        "Invalid path - too many parts",
			path:        "workspace/subdir/repo.git",
			wantErr:     true,
			errContains: "invalid repository path format",
		},
		{
			name:        "Invalid path - only workspace",
			path:        "workspace",
			wantErr:     true,
			errContains: "invalid repository path format",
		},
		{
			name:        "Invalid path - empty",
			path:        "",
			wantErr:     true,
			errContains: "invalid repository path format",
		},
		{
			name:        "Invalid repository name - starts with dot",
			path:        "workspace/.repo",
			wantErr:     true,
			errContains: "invalid repository name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotWorkspace, gotRepo, err := parseRepositoryPath(tt.path)

			if tt.wantErr {
				if err == nil {
					t.Errorf("parseRepositoryPath() expected error, got nil")
					return
				}
				if tt.errContains != "" && err != nil {
					if !strings.Contains(err.Error(), tt.errContains) {
						t.Errorf("parseRepositoryPath() error = %v, want error containing %q", err, tt.errContains)
					}
				}
				return
			}

			if err != nil {
				t.Errorf("parseRepositoryPath() unexpected error = %v", err)
				return
			}

			if gotWorkspace != tt.wantWorkspace {
				t.Errorf("parseRepositoryPath() workspace = %v, want %v", gotWorkspace, tt.wantWorkspace)
			}

			if gotRepo != tt.wantRepo {
				t.Errorf("parseRepositoryPath() repo = %v, want %v", gotRepo, tt.wantRepo)
			}
		})
	}
}
