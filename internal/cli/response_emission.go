package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/DiLRandI/OTelPlan/internal/resolve"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func usageError(opts options, command, message string, stdout, stderr io.Writer) int {
	if opts.format != jsonFormat {
		_, err := fmt.Fprintln(stderr, message)
		if err != nil {
			return 1
		}

		return exitUsage
	}

	var diagnostic model.DiagnosticError

	diagnostic.Severity, diagnostic.Code, diagnostic.Message = model.SeverityError, model.CodeInvalidPolicy, message
	reply := response{
		APIVersion: APIVersion, Command: command, OK: false,
		Diagnostics: model.DiagnosticErrorList{diagnostic}, Data: nil, Details: nil,
	}

	err := emit(stdout, opts, reply)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)

		return 1
	}

	return exitUsage
}

func emit(out io.Writer, opts options, reply response) error {
	reply = prepareResponse(opts, reply)

	if opts.format == jsonFormat {
		return emitJSONResponse(out, reply)
	}

	for _, diagnostic := range reply.Diagnostics {
		_, err := fmt.Fprintln(out, diagnostic.Error())
		if err != nil {
			return fmt.Errorf("write diagnostic: %w", err)
		}
	}

	err := emitDiagnosticDetails(out, opts)
	if err != nil {
		return err
	}

	if opts.quiet {
		return nil
	}

	var text strings.Builder

	renderResponseText(&text, opts, reply.Data)

	_, err = io.WriteString(out, text.String())
	if err != nil {
		return fmt.Errorf("write text response: %w", err)
	}

	return nil
}

func prepareResponse(opts options, reply response) response {
	if inventory, ok := reply.Data.(*model.CodeModel); ok {
		reply.Data = previewInventory(inventory)
	}

	if opts.details != nil && (opts.details.Build != nil || len(opts.details.Failures) > 0) {
		reply.Details = opts.details
	}

	return reply
}

func emitJSONResponse(out io.Writer, reply response) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")

	err := encoder.Encode(reply)
	if err != nil {
		return fmt.Errorf("encode JSON response: %w", err)
	}

	return nil
}

func renderResponseText(text *strings.Builder, opts options, data any) {
	switch contents := data.(type) {
	case string:
		fmt.Fprintln(text, contents)
	case *model.CodeModel:
		renderInventory(text, opts, contents)
	case model.ResolvedPlan:
		renderPlan(text, contents)
	case resolve.Explanation:
		fmt.Fprintf(text, "%s selected=%t\n", contents.SymbolID, contents.Selected)

		for _, decision := range contents.Decisions {
			fmt.Fprintf(text, "  %s %s: %s\n", decision.RuleID, decision.Stage, decision.Reason)
		}
	default:
		renderCommandSummary(text, data)
	}
}

func renderInventory(text *strings.Builder, opts options, code *model.CodeModel) {
	for _, symbol := range code.Symbols {
		fmt.Fprintf(text, "%s\n  signature %s\n  source %s:%d\n  context %v  errors %v\n",
			symbol.ID, symbol.Signature, symbol.Location.File, symbol.Location.Line,
			symbol.ContextIndexes, symbol.ErrorIndexes)
	}

	renderAdvisoryCalls(text, code)

	if opts.interfaces {
		for _, binding := range code.InterfaceMethods {
			fmt.Fprintf(text, "IMPLEMENTS %s %s\n", binding.InterfaceID, binding.SymbolID)
		}
	}
}

func renderAdvisoryCalls(text *strings.Builder, code *model.CodeModel) {
	if code.CallGraph == nil {
		return
	}

	fmt.Fprintf(text, "CALLGRAPH %s conservative=%t scope=%s\n",
		code.CallGraph.Algorithm, code.CallGraph.Conservative, code.CallGraph.Scope)

	for _, limitation := range code.CallGraph.Limitations {
		fmt.Fprintf(text, "  limitation: %s\n", limitation)
	}

	for _, edge := range code.CallEdges {
		fmt.Fprintf(text, "CALL %s %s -> %s\n", edge.Precision, edge.Caller, edge.Callee)
	}
}

func renderPlan(text *strings.Builder, plan model.ResolvedPlan) {
	for _, target := range plan.Targets {
		fmt.Fprintf(text, "SELECTED %s\n  span %s\n  context %s[%d]\n  errors record=%t indexes=%v\n  rule %s\n",
			target.SymbolID, target.SpanName, target.ContextStrategy.Strategy, target.ContextStrategy.Index,
			target.ErrorStrategy.Record, target.ErrorStrategy.Indexes, target.RuleID)

		for _, attribute := range target.Attributes {
			fmt.Fprintf(text, "  attribute %s from %s\n", attribute.Key, attributeSourceText(attribute))
		}
	}

	for _, skipped := range plan.Skipped {
		fmt.Fprintf(text, "SKIPPED %s\n  rule %s: %s\n", skipped.SymbolID, skipped.RuleID, skipped.Reason)
	}
}

func attributeSourceText(attribute model.AttributePlan) string {
	if attribute.From.Result != "" {
		return "result " + attribute.From.Result
	}

	if attribute.From.Argument != "" {
		return "argument " + attribute.From.Argument
	}

	return "constant"
}

func renderCommandSummary(text *strings.Builder, data any) {
	switch contents := data.(type) {
	case buildSummary:
		renderBuildSummary(text, contents)
	case initSummary:
		renderInitSummary(text, contents)
	case compileSummary:
		fmt.Fprintf(text, "%s artifacts=%d\n", contents.Path, contents.Files)
	case lockSummary:
		fmt.Fprintf(text, "%s targets=%d changed=%t dry-run=%t\n",
			contents.Path, contents.Targets, contents.Changed, contents.DryRun)
	case model.LockDiff:
		for _, entry := range contents.Entries {
			fmt.Fprintf(text, "%s %s %s\n", entry.Classification, entry.Symbol, entry.Detail)
		}
	case map[string]string:
		fmt.Fprintf(text, "otelplan %s\nGo %s\n", contents["otelplan"], contents["go"])
	}
}

func renderBuildSummary(text *strings.Builder, summary buildSummary) {
	if summary.Path != "" {
		fmt.Fprintf(text, "built %s %s\n", summary.Path, summary.Digest)
	}

	for _, file := range summary.Files {
		fmt.Fprintf(text, "built %s %s\n", file.Path, file.Digest)
	}

	if summary.Path == "" && len(summary.Files) == 0 {
		fmt.Fprintln(text, "build succeeded; no executable output")
	}
}

func renderInitSummary(text *strings.Builder, summary initSummary) {
	fmt.Fprintf(text, "wrote %s with %d suggested rule(s)\n", summary.Path, len(summary.Suggestions))

	for _, candidate := range summary.Suggestions {
		fmt.Fprintf(text, "SUGGESTED %s confidence=%s score=%d\n",
			candidate.SymbolID, candidate.Confidence, candidate.Score)
	}
}
