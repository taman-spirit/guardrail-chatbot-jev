// Package guardrail puts content guardrails in front of an AI chatbot, decided by the Jev
// decision model.
//
//	guard := guardrail.New(guardrail.Options{})
//	verdict, err := guard.CheckInput(ctx, userMessage, nil)
//	if err != nil {
//		return err
//	}
//	if !verdict.Allowed() {
//		return safeResponse(verdict)
//	}
//
// This is a port of the Python and TypeScript packages in the same repository. It reads the same
// policy pack, sends the same request, and reaches the same verdict for the same answers. Session
// state written by one language can be read by the others.
package guardrail

// Version is the Go module version. It moves ahead of the Python and TypeScript packages when the
// Go module ships something they have not released yet.
const Version = "1.1.0"
