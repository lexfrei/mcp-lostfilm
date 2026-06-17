package tools

import "github.com/modelcontextprotocol/go-sdk/mcp"

// ptrBool returns a pointer to b, for the *bool annotation hint fields.
func ptrBool(value bool) *bool { return &value }

// readOnly builds annotations for a tool that only reads remote state.
func readOnly(title string) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		Title:         title,
		ReadOnlyHint:  true,
		OpenWorldHint: ptrBool(true),
	}
}
