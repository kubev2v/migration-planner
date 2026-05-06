package cli

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func NewCmdRelease() *cobra.Command {
	var tag string
	var remote string
	var repoPath string
	var sha string
	var branch string

	cmd := &cobra.Command{
		Use:   "release",
		Short: "Automate the release process by creating a tag and GitHub release",
		Long: `Create a Git tag, push it to the remote repository, and create a GitHub release.

This command requires the GitHub CLI (gh) to be installed and authenticated.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRelease(tag, remote, repoPath, sha, branch)
		},
	}

	cmd.Flags().StringVar(&tag, "tag", "", "Tag/version to create (required)")
	cmd.Flags().StringVar(&sha, "sha", "", "Head Sha to create tag from")
	cmd.Flags().StringVar(&branch, "branch", "main", "Branch to create tag from")
	cmd.Flags().StringVar(&remote, "remote", "upstream", "Git remote to push the tag to")
	cmd.Flags().StringVar(&repoPath, "repo-path", ".", "Path to the Git repository")
	_ = cmd.MarkFlagRequired("tag")

	return cmd
}

func runRelease(tag, remote, repoPath, sha, branch string) error {
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return fmt.Errorf("failed to resolve repository path: %w", err)
	}

	if err := validateRemote(absPath, remote); err != nil {
		return err
	}

	if err := checkGHInstalled(); err != nil {
		return err
	}

	if err := fetchBranch(repoPath, remote, branch); err != nil {
		return err
	}

	if sha == "" {
		headSHA, err := getRemoteHeadSHA(absPath, remote, branch)
		if err != nil {
			return err
		}
		sha = headSHA
	}

	repoName, err := getGitHubRepo(absPath, remote)
	if err != nil {
		return err
	}

	confirm, err := confirmRelease(tag, repoName)
	if err != nil {
		return err
	}

	if !confirm {
		fmt.Println("Release cancelled.")
		return nil
	}

	if err := createTag(absPath, tag, sha); err != nil {
		return err
	}
	fmt.Printf("✓ Created tag %s at commit %s\n", tag, sha)

	if err := pushTag(absPath, remote, tag); err != nil {
		return err
	}
	fmt.Printf("✓ Pushed tag %s to %s\n", tag, remote)

	if err := createGitHubRelease(tag, repoName); err != nil {
		return err
	}
	fmt.Printf("✓ Created GitHub release: Release %s\n", tag)

	fmt.Printf("\nHEAD SHA: %s\n", sha)

	return nil
}

func validateRemote(repoPath, remote string) error {
	cmd := exec.Command("git", "remote", "get-url", remote)
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("remote '%s' not found in repository", remote)
	}
	return nil
}

func getRemoteHeadSHA(repoPath, remote, branch string) (string, error) {
	cmd := exec.Command("git", "ls-remote", remote, fmt.Sprintf("refs/heads/%s", branch))
	cmd.Dir = repoPath

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get remote HEAD SHA: %w", err)
	}

	// Output format: "<sha>\trefs/heads/<branch>\n"
	parts := strings.Fields(string(output))
	if len(parts) == 0 {
		return "", fmt.Errorf("unexpected output from git ls-remote")
	}

	return parts[0], nil
}

func checkGHInstalled() error {
	cmd := exec.Command("gh", "--version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("'gh' command not found. Please install GitHub CLI: https://cli.github.com/")
	}
	return nil
}

func getGitHubRepo(repoPath, remote string) (string, error) {
	cmd := exec.Command("git", "remote", "get-url", remote)
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get remote URL: %w", err)
	}

	remoteURL := strings.TrimSpace(string(output))

	// Parse GitHub repo from URL
	// Handles both SSH (git@github.com:owner/repo.git) and HTTPS (https://github.com/owner/repo.git)
	var repo string
	if strings.Contains(remoteURL, "github.com") {
		parts := strings.Split(remoteURL, "github.com")
		if len(parts) == 2 {
			repo = strings.TrimPrefix(parts[1], ":")
			repo = strings.TrimPrefix(repo, "/")
			repo = strings.TrimSuffix(repo, ".git")
		}
	}

	if repo == "" {
		return "", fmt.Errorf("could not parse GitHub repository from remote URL: %s", remoteURL)
	}

	return repo, nil
}

func confirmRelease(tag, repo string) (bool, error) {
	fmt.Printf("Note: You're going to create a release %s for repo %s. Do you want to proceed? (Y/N): ", tag, repo)

	var response string
	if _, err := fmt.Scanln(&response); err != nil {
		return false, err
	}

	response = strings.ToUpper(strings.TrimSpace(response))
	return response == "Y" || response == "YES", nil
}

func createTag(repoPath, tag, sha string) error {
	cmd := exec.Command("git", "tag", "-a", tag, "-m", fmt.Sprintf("Release %s", tag), sha)
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "already exists") {
			return fmt.Errorf("tag '%s' already exists. Use 'git tag -d %s' to delete it first", tag, tag)
		}
		return fmt.Errorf("failed to create tag: %w\n%s", err, string(output))
	}
	return nil
}

func fetchBranch(repoPath, remote, branch string) error {
	cmd := exec.Command("git", "fetch", remote, branch)
	cmd.Dir = repoPath
	return cmd.Run()
}

func pushTag(repoPath, remote, tag string) error {
	cmd := exec.Command("git", "push", remote, tag)
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to push tag to remote '%s': %w\n%s", remote, err, string(output))
	}
	return nil
}

func createGitHubRelease(tag, repo string) error {
	cmd := exec.Command("gh", "release", "create", tag,
		"--title", fmt.Sprintf("Release %s", tag),
		"--repo", repo)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create GitHub release: %w\n%s", err, string(output))
	}
	return nil
}
