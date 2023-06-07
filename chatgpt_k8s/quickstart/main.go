package main

import (
	"context"
	"fmt"
	gogpt "github.com/sashabaranov/go-gpt3"
	"os"
)

func main() {
	token := os.Getenv("OPENAI_API_KEY")
	c := gogpt.NewClient(token)
	ctx := context.Background()

	req := gogpt.CompletionRequest{
		Model:     gogpt.GPT3TextAda001,
		MaxTokens: 500,
		Prompt:    "什么是 Kubernetes?",
	}
	resp, err := c.CreateCompletion(ctx, req)
	if err != nil {
		return
	}
	fmt.Println(resp.Choices[0].Text)
}
