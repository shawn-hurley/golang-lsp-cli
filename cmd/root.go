package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/shawn-hurley/golang-lsp-cli/pkg/jsonrpc2"
	"github.com/shawn-hurley/golang-lsp-cli/pkg/lsp/protocol"
	"github.com/spf13/cobra"
)

var method string
var jsonData map[string]interface{}

var rootCmd = &cobra.Command{
	Use:   "lsp-cli",
	Short: "lsp-cli - a simple CLI for talking to LSP Servers",
	Long: `lsp-cli - a simple CLI for talking to LSP Servers
   
useful for prototyping what a language server will return based on the call sent.

example usage:

lsp-cli workspace/symbol '{"query": "*"}'

`,
	ValidArgs: []string{"method_name", "json_struct"},
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) != 2 {
			return fmt.Errorf("must pass in method name, and json struct")
		}

		method = args[0]
		jsonString := args[1]

		err := json.Unmarshal([]byte(jsonString), &jsonData)
		if err != nil {
			return err
		}

		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {

		ctx := context.Background()

		var languageServer *exec.Cmd
		if len(languageServerCMD) > 1 {
			languageServer = exec.CommandContext(ctx, languageServerCMD[0], languageServerCMD[1:]...)
		} else {
			languageServer = exec.CommandContext(ctx, languageServerCMD[0])
		}

		stdin, err := languageServer.StdinPipe()
		if err != nil {
			fmt.Printf("error: %v", err)
			return
		}
		stdout, err := languageServer.StdoutPipe()
		if err != nil {
			fmt.Printf("error: %v", err)
			return
		}

		calls := &bytes.Buffer{}
		newWriter := io.MultiWriter(calls, stdin)

		go func() {
			err := languageServer.Run()
			if err != nil {
				fmt.Printf("language server failed to run- %v", err)
			}
		}()

		// Connect the header stream to the in and out
		rpc := jsonrpc2.NewConn(jsonrpc2.NewHeaderStream(stdout, newWriter))

		go func() {
			err := rpc.Run(ctx)
			if err != nil {
				fmt.Printf("connection terminated: %v", err)
			}
		}()

		if codeLocation == "" {
			codeLocation, err = os.Getwd()
			if err != nil {
				fmt.Printf("failed to call method: %s: %v", method, err)
			}
		}
		params := &protocol.InitializeParams{
			//TODO(shawn-hurley): add ability to parse path to URI in a real supported way
			RootURI: fmt.Sprintf("file://%v", codeLocation),
			Capabilities: protocol.ClientCapabilities{
				TextDocument: protocol.TextDocumentClientCapabilities{
					DocumentSymbol: &protocol.DocumentSymbolClientCapabilities{
						HierarchicalDocumentSymbolSupport: true,
					},
				},
			},
			ExtendedClientCapilities: map[string]interface{}{
				"classFileContentsSupport": true,
			},
		}

		// Initalize the client and server connection, must be done before every call
		var result protocol.InitializeResult
		for {
			if err := rpc.Call(ctx, "initialize", params, &result); err != nil {
				fmt.Printf("waiting to initialize - failed: %v", err)
				continue
			}
			break
		}
		if err := rpc.Notify(ctx, "initialized", &protocol.InitializedParams{}); err != nil {
			fmt.Printf("initialized failed: %v\n", err)
			return
		}

		var res interface{}
		err = rpc.Call(ctx, method, jsonData, &res)
		if err != nil {
			fmt.Printf("failed to call method: %s: %v", method, err)
			return
		}

		resString, err := json.Marshal(res)
		if err != nil {
			fmt.Printf("unable to understand output")
			return
		}
		fmt.Printf("%q", resString)

		if printMessageSTDOUT {
			fmt.Printf("\n\nCalls\n%q", calls)
		}
	},
}

var printMessageSTDOUT = false
var languageServerCMD = []string{"gopls"}
var codeLocation = ""

func init() {
	rootCmd.Flags().BoolVarP(&printMessageSTDOUT, "verbose", "v", false, "prints message to stdout")
	rootCmd.Flags().StringArrayVarP(&languageServerCMD, "language-server", "s", []string{"gopls"}, "abosolute path to a langague server runnable")
	rootCmd.Flags().StringVarP(&codeLocation, "data", "d", "", "Will default to the current working directory")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "There was an error: '%s'", err)
		os.Exit(1)
	}
}
