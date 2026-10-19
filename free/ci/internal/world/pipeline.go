package world

// Purpose: tells a Contributor which pipeline the world is being built for.
// Inputs:  WithPipeline(ctx, id), applied by Build before it runs contributors.
// Outputs: PipelineID(ctx), the id or "" when the context names none.
// Constraints: a contributor that reads per-attempt data restricts it to this
// pipeline, so unrelated pipelines neither change the world digest nor add work.

import "context"

type pipelineKey struct{}

// WithPipeline returns ctx carrying the id of the pipeline being built.
func WithPipeline(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, pipelineKey{}, id)
}

// PipelineID is the pipeline a contributor builds for, "" when unknown.
func PipelineID(ctx context.Context) string {
	id, _ := ctx.Value(pipelineKey{}).(string)
	return id
}
