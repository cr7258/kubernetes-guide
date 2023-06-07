package main

import (
	"context"
	"fmt"
	gogpt "github.com/sashabaranov/go-gpt3"
	"log"
	"os"
)

func main() {
	qList := []string{
		"生成一个 k8s pod yaml",
		"请把副本调整为5",
	}
	qText := ""
	for _, q := range qList {
		qText += "\n" + q
		ret, err := SimpleChat(qText)
		if err != nil {
			log.Fatalln(err)
		}
		fmt.Println(q)
		fmt.Println(ret)
		qText += " " + ret
	}
}

func SimpleChat(promt string) (string, error) {
	token := os.Getenv("OPENAI_API_KEY")
	c := gogpt.NewClient(token)
	ctx := context.Background()

	req := gogpt.CompletionRequest{
		Model:     gogpt.GPT3TextDavinci003,
		MaxTokens: 2500,
		Prompt:    promt,
	}

	resp, err := c.CreateCompletion(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.Choices[0].Text, nil
}
