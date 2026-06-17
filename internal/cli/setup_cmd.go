package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var skillURL = "https://raw.githubusercontent.com/strausslabs/pando/main/docs/pando-star-skill/SKILL.md"

func setupCmd(g *globalFlags) *cobra.Command {
	var skipMCP, skipSkill, global bool
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Install the pando.star skill and register the MCP server for AI agents",
		RunE: func(c *cobra.Command, args []string) error {
			self, err := os.Executable()
			if err != nil {
				return err
			}
			if !skipSkill {
				if err := installSkill(c.Context(), global); err != nil {
					return err
				}
			}
			if !skipMCP {
				registerMCP(self, global)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&skipMCP, "no-mcp", false, "don't register the MCP server")
	cmd.Flags().BoolVar(&skipSkill, "no-skill", false, "don't install the pando.star skill")
	cmd.Flags().BoolVar(&global, "global", false, "install for every project (~/.claude + claude user scope) instead of just this one")
	return cmd
}

func installSkill(ctx context.Context, global bool) error {
	base := ".claude"
	if global {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		base = filepath.Join(home, ".claude")
	}
	doc, err := fetchSkill(ctx)
	if err != nil {
		return fmt.Errorf("download skill: %w", err)
	}
	dir := filepath.Join(base, "skills", "pando-star")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, doc, 0o644); err != nil {
		return err
	}
	fmt.Printf("installed skill → %s\n", path)
	return nil
}

func fetchSkill(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, skillURL, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func registerMCP(self string, global bool) {
	args := []string{"mcp", "add", "pando"}
	if global {
		args = append(args, "--scope", "user")
	}
	args = append(args, "--", self, "mcp")
	manual := "claude " + strings.Join(args, " ")

	claude, err := exec.LookPath("claude")
	if err != nil {
		fmt.Println("claude CLI not found; register the MCP server yourself with:")
		fmt.Printf("  %s\n", manual)
		return
	}
	cmd := exec.Command(claude, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("could not register MCP automatically; run: %s\n", manual)
		return
	}
	fmt.Println("registered MCP server 'pando'")
}
