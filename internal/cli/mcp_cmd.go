package cli

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"github.com/strausslabs/pando/internal/mcpserver"
)

func mcpCmd(g *globalFlags, version string) *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Run the Pando MCP server (stdio) for AI agents",
		Long: "Speak the Model Context Protocol over stdio so an agent can drive Pando.\n" +
			"Register with: claude mcp add pando -- pando mcp",
		RunE: func(c *cobra.Command, args []string) error {
			srv := mcpserver.NewServer(version, nil)
			return srv.Run(c.Context(), &mcp.StdioTransport{})
		},
	}
}
