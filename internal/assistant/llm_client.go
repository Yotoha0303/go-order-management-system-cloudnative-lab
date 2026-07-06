package assistant

import "context"

type LLMClient interface {
	ParseIntent(ctx context.Context, message string) (IntentResult, LLMUsage, error)
}
