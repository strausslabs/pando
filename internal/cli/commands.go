package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/guyStrauss/pando/internal/api"
	"github.com/guyStrauss/pando/internal/client"
	"github.com/spf13/cobra"
)

// resolveWorktree picks the target worktree slug. Explicit --worktree wins;
// then PANDO_WORKTREE; otherwise it asks the daemon for its registered
// worktrees and matches the one containing the current directory, falling back
// to the sole worktree when only one is registered.
func resolveWorktree(cl *client.Client, flag string) (string, error) {
	if flag != "" {
		return flag, nil
	}
	if env := os.Getenv("PANDO_WORKTREE"); env != "" {
		return env, nil
	}
	wts, err := cl.ListWorktrees(ctx())
	if err != nil {
		return "", err
	}
	if len(wts) == 1 {
		return wts[0].Slug, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for _, w := range wts {
		if pathContains(w.Path, cwd) {
			return w.Slug, nil
		}
	}
	return "", fmt.Errorf("could not determine current worktree; pass --worktree")
}

func pathContains(parent, child string) bool {
	p, err1 := filepath.Abs(parent)
	c, err2 := filepath.Abs(child)
	if err1 != nil || err2 != nil {
		return false
	}
	rel, err := filepath.Rel(p, c)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func upCmd(g *globalFlags) *cobra.Command {
	var worktree string
	var force bool
	cmd := &cobra.Command{
		Use:   "up",
		Short: "Bring the stack up for a worktree",
		RunE: func(c *cobra.Command, args []string) error {
			cl := client.New(g.socket)
			wt, err := resolveWorktree(cl, worktree)
			if err != nil {
				return err
			}
			return cl.Up(ctx(), wt, force)
		},
	}
	cmd.Flags().StringVarP(&worktree, "worktree", "w", "", "target worktree slug")
	cmd.Flags().BoolVar(&force, "force", false, "re-run run-once tasks")
	return cmd
}

func downCmd(g *globalFlags) *cobra.Command {
	var worktree string
	cmd := &cobra.Command{
		Use:   "down",
		Short: "Tear the stack down for a worktree",
		RunE: func(c *cobra.Command, args []string) error {
			cl := client.New(g.socket)
			wt, err := resolveWorktree(cl, worktree)
			if err != nil {
				return err
			}
			return cl.Down(ctx(), wt)
		},
	}
	cmd.Flags().StringVarP(&worktree, "worktree", "w", "", "target worktree slug")
	return cmd
}

func statusCmd(g *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show status of all worktrees and resources",
		RunE: func(c *cobra.Command, args []string) error {
			cl := client.New(g.socket)
			st, err := cl.Status(ctx())
			if err != nil {
				return err
			}
			if g.json {
				return printJSON(st)
			}
			printStatus(st)
			return nil
		},
	}
}

func printStatus(st []api.WorktreeStatus) {
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "WORKTREE\tRESOURCE\tKIND\tPHASE\tPORT")
	for _, ws := range st {
		for _, r := range ws.Resources {
			port := ""
			if r.Port > 0 {
				port = fmt.Sprintf("%d", r.Port)
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", ws.Worktree, r.Name, r.Kind, r.Phase, port)
		}
	}
	_ = tw.Flush()
}

func logsCmd(g *globalFlags) *cobra.Command {
	var worktree, grep string
	var tail int
	cmd := &cobra.Command{
		Use:   "logs <resource>",
		Short: "Show logs for a resource",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cl := client.New(g.socket)
			wt, err := resolveWorktree(cl, worktree)
			if err != nil {
				return err
			}
			lines, err := cl.Logs(ctx(), api.LogQuery{
				Worktree: wt,
				Resource: args[0],
				Tail:     tail,
				Grep:     grep,
			})
			if err != nil {
				return err
			}
			if g.json {
				return printJSON(lines)
			}
			for _, l := range lines {
				fmt.Printf("%s\n", l.Text)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&worktree, "worktree", "w", "", "target worktree slug")
	cmd.Flags().IntVar(&tail, "tail", 200, "number of trailing lines")
	cmd.Flags().StringVar(&grep, "grep", "", "filter lines by regex")
	return cmd
}

func execCmd(g *globalFlags) *cobra.Command {
	var worktree string
	cmd := &cobra.Command{
		Use:   "exec <resource> -- <cmd>...",
		Short: "Run a command inside a resource",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			cl := client.New(g.socket)
			wt, err := resolveWorktree(cl, worktree)
			if err != nil {
				return err
			}
			res, err := cl.Exec(ctx(), api.ExecRequest{
				Worktree: wt,
				Resource: args[0],
				Cmd:      args[1:],
			})
			if err != nil {
				return err
			}
			if g.json {
				if err := printJSON(res); err != nil {
					return err
				}
			} else {
				fmt.Fprint(os.Stdout, res.Stdout)
				fmt.Fprint(os.Stderr, res.Stderr)
			}
			if res.ExitCode != 0 {
				os.Exit(res.ExitCode)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&worktree, "worktree", "w", "", "target worktree slug")
	return cmd
}

func restartCmd(g *globalFlags) *cobra.Command {
	var worktree string
	cmd := &cobra.Command{
		Use:   "restart <resource>",
		Short: "Restart a single resource and its dependents",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cl := client.New(g.socket)
			wt, err := resolveWorktree(cl, worktree)
			if err != nil {
				return err
			}
			return cl.Restart(ctx(), wt, args[0])
		},
	}
	cmd.Flags().StringVarP(&worktree, "worktree", "w", "", "target worktree slug")
	return cmd
}

func worktreesCmd(g *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "worktrees",
		Short: "List registered worktrees and their ports",
		RunE: func(c *cobra.Command, args []string) error {
			cl := client.New(g.socket)
			wts, err := cl.ListWorktrees(ctx())
			if err != nil {
				return err
			}
			if g.json {
				return printJSON(wts)
			}
			tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
			fmt.Fprintln(tw, "SLUG\tBRANCH\tPATH")
			for _, w := range wts {
				fmt.Fprintf(tw, "%s\t%s\t%s\n", w.Slug, w.Branch, w.Path)
			}
			_ = tw.Flush()
			return nil
		},
	}
}
