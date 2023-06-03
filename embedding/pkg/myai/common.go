package myai

import (
	"context"
	openai "github.com/sashabaranov/go-openai"
	"log"
	"net/http"
	"net/url"
	"os"
)

// 设置自己的科学代理地址
func myProxyTransport() *http.Transport {
	SocksProxy := "socks5://127.0.0.1:4781"

	uri, err := url.Parse(SocksProxy)
	if err != nil {
		log.Fatalln(err)
	}
	return &http.Transport{
		Proxy: http.ProxyURL(uri),
	}
}

func NewOpenAiClient() *openai.Client {
	token := os.Getenv("OPENAI_API_KEY")
	config := openai.DefaultConfig(token)

	// 如果需要通过代理访问请设置
	//config.HTTPClient.Transport = myProxyTransport()
	return openai.NewClientWithConfig(config)
}

// 快速函数，把 搜索词变成向量
func SimpleGetVec(prompt string) ([]float32, error) {
	c := NewOpenAiClient()
	req := openai.EmbeddingRequest{
		Input: []string{prompt},
		Model: openai.AdaEmbeddingV2,
	}
	rsp, err := c.CreateEmbeddings(context.TODO(), req)
	if err != nil {
		return nil, err
	}
	return rsp.Data[0].Embedding, nil
}
