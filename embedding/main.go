package main

import (
	"fmt"
	pb "github.com/qdrant/go-client/qdrant"
	"log"
	"myembedding/pkg/helpers/qdrant"
	"myembedding/pkg/myai"
	"strings"
)

func parseAnswers(size int, old *pb.ListValue) string {
	if len(old.GetValues()) == 0 {
		log.Println("没有参考答案")
		return "none"
	}
	newAnswer := ""
	for i := 0; i < size; i++ {
		newAnswer += old.GetValues()[i].GetStringValue() + "\n"
	}
	alist := strings.Split(newAnswer, "\n")
	ret := ""
	for _, v := range alist {
		if v != "" {
			ret += v + "\n"
		}
	}
	ret = strings.TrimSpace(ret)
	if ret == "" {
		ret = "none"
	}
	return ret
}

func main() {

	prompt := "kubectl 镜像 策略"
	p_vec, err := myai.SimpleGetVec(prompt)
	if err != nil {
		panic(err)
	}
	points, err := qdrant.FastQdrantClient.Search("kubernetes", p_vec)
	if err != nil {
		panic(err)
	}

	if points[0].Score < 0.8 {
		fmt.Println("没有找到答案")
		return
	}
	answer := parseAnswers(2, points[0].Payload["answers"].GetListValue())

	tmpl := "question: %s\n" + "description: %s\n" + "reference answer: %s\n"

	finalPrompt := fmt.Sprintf(tmpl, prompt, points[0].Payload["question"].GetStringValue(), answer)
	fmt.Println(finalPrompt)
	fmt.Println("------------------------")
	fmt.Println(myai.K8sChat(finalPrompt))

}
