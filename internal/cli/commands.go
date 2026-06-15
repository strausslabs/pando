package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/strausslabs/pando/internal/api"
	"github.com/strausslabs/pando/internal/client"
)

func addWorktreeFlag(cmd *cobra.Command, target *string) {
	cmd.Flags().StringVarP(target, "worktree", "w", "", "target worktree slug")
}

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

func resolveWorktreeWait(cl *client.Client, flag string, timeout time.Duration) (string, error) {
	if flag != "" {
		return flag, nil
	}
	if env := os.Getenv("PANDO_WORKTREE"); env != "" {
		return env, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	deadline := time.Now().Add(timeout)
	for {
		wts, err := cl.ListWorktrees(ctx())
		if err != nil {
			return "", err
		}
		for _, w := range wts {
			if pathContains(w.Path, cwd) {
				return w.Slug, nil
			}
		}
		if time.Now().After(deadline) {
			if len(wts) == 1 {
				return wts[0].Slug, nil
			}
			return "", fmt.Errorf("could not determine current worktree; pass --worktree")
		}
		time.Sleep(150 * time.Millisecond)
	}
}

func pathContains(parent, child string) bool {
	p, err1 := canonPath(parent)
	c, err2 := canonPath(child)
	if err1 != nil || err2 != nil {
		return false
	}
	rel, err := filepath.Rel(p, c)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func canonPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved, nil
	}
	return abs, nil
}

func upCmd(g *globalFlags) *cobra.Command {
	var worktree string
	var force bool
	cmd := &cobra.Command{
		Use:   "up",
		Short: "Bring the stack up for a worktree (starts the daemon + dashboard if needed)",
		RunE: func(c *cobra.Command, args []string) error {
			cl, err := ensureClient(g)
			if err != nil {
				return err
			}
			wt, err := resolveWorktreeWait(cl, worktree, 10*time.Second)
			if err != nil {
				return err
			}
			if err := cl.Up(ctx(), wt, force); err != nil {
				return err
			}
			if g.json {
				st, err := cl.Status(ctx())
				if err != nil {
					return err
				}
				return printJSON(worktreeIn(st, wt))
			}
			fmt.Printf("%s is up\n", wt)
			if st, err := cl.Status(ctx()); err == nil {
				printStatus(worktreeIn(st, wt))
			}
			return nil
		},
	}
	addWorktreeFlag(cmd, &worktree)
	cmd.Flags().BoolVar(&force, "force", false, "re-run run-once tasks")
	return cmd
}

func worktreeIn(st []api.WorktreeStatus, slug string) []api.WorktreeStatus {
	for _, ws := range st {
		if ws.Worktree == slug {
			return []api.WorktreeStatus{ws}
		}
	}
	return nil
}

func downCmd(g *globalFlags) *cobra.Command {
	var worktree string
	cmd := &cobra.Command{
		Use:   "down",
		Short: "Tear the stack down for a worktree",
		RunE: func(c *cobra.Command, args []string) error {
			cl, err := newClient(g)
			if err != nil {
				return err
			}
			wt, err := resolveWorktree(cl, worktree)
			if err != nil {
				return err
			}
			return cl.Down(ctx(), wt)
		},
	}
	addWorktreeFlag(cmd, &worktree)
	return cmd
}

func statusCmd(g *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show status of all worktrees and resources",
		RunE: func(c *cobra.Command, args []string) error {
			cl, err := newClient(g)
			if err != nil {
				return err
			}
			st, err := cl.Status(ctx())
			if err != nil {
				return err
			}
			if g.json {
				return printJSON(st)
			}
			printStatus(st)
			if up, err := cl.Version(ctx()); err == nil && up.Available {
				fmt.Fprintf(os.Stderr, "\n"+updateAvailableMsg, up.Current, up.Latest)
			}
			return nil
		},
	}
}

func printStatus(st []api.WorktreeStatus) {
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "WORKTREE\tRESOURCE\tKIND\tPHASE\tPORT")
	for _, ws := range st {
		if ws.Error != "" {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", ws.Worktree, "", "", "config error", "")
		}
		for _, r := range ws.Resources {
			port := ""
			if r.Port > 0 {
				port = fmt.Sprintf("%d", r.Port)
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", ws.Worktree, r.Name, r.Kind, r.Phase, port)
		}
	}
	_ = tw.Flush()
	for _, ws := range st {
		if ws.Error != "" {
			_, _ = fmt.Fprintf(os.Stderr, "config error in %s: %s\n", ws.Worktree, ws.Error)
		}
	}
}

func logsCmd(g *globalFlags) *cobra.Command {
	var worktree, grep string
	var tail int
	cmd := &cobra.Command{
		Use:   "logs <resource>",
		Short: "Show logs for a resource",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cl, err := newClient(g)
			if err != nil {
				return err
			}
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
	addWorktreeFlag(cmd, &worktree)
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
			cl, err := newClient(g)
			if err != nil {
				return err
			}
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
				_, _ = fmt.Fprint(os.Stdout, res.Stdout)
				_, _ = fmt.Fprint(os.Stderr, res.Stderr)
			}
			if res.ExitCode != 0 {
				os.Exit(res.ExitCode)
			}
			return nil
		},
	}
	addWorktreeFlag(cmd, &worktree)
	return cmd
}

func restartCmd(g *globalFlags) *cobra.Command {
	var worktree string
	cmd := &cobra.Command{
		Use:   "restart <resource>",
		Short: "Restart a single resource and its dependents",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cl, err := newClient(g)
			if err != nil {
				return err
			}
			wt, err := resolveWorktree(cl, worktree)
			if err != nil {
				return err
			}
			return cl.Restart(ctx(), wt, args[0])
		},
	}
	addWorktreeFlag(cmd, &worktree)
	return cmd
}

func worktreesCmd(g *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "worktrees",
		Short: "List registered worktrees and their ports",
		RunE: func(c *cobra.Command, args []string) error {
			cl, err := newClient(g)
			if err != nil {
				return err
			}
			wts, err := cl.ListWorktrees(ctx())
			if err != nil {
				return err
			}
			if g.json {
				return printJSON(wts)
			}
			tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "SLUG\tBRANCH\tPATH")
			for _, w := range wts {
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", w.Slug, w.Branch, w.Path)
			}
			_ = tw.Flush()
			return nil
		},
	}
}
