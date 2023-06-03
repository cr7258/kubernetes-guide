package main

import (
	"fmt"
	pb "github.com/qdrant/go-client/qdrant"
	"log"
	"myembedding/pkg/helpers/qdrant"
)

var urls = []string{
	"https://stackoverflow.com/questions/42564058/how-to-use-local-docker-images-with-minikube",
	"https://stackoverflow.com/questions/59980445/setting-image-pull-policy-using-kubectl",
	"https://stackoverflow.com/questions/47909256/how-to-use-apply-instead-of-create-for-deployment-in-kubernetes",
}

func main() {

	// 判断 collection 是否存在
	collectionExist, err := qdrant.HasCollection("kubernetes")
	if err != nil {
		log.Fatalln("获取collection出错:", err.Error())
	}

	if collectionExist == false {
		//创建 collection:  kubernetes
		err := qdrant.Collection("kubernetes").Create(1536)
		if err != nil {
			log.Fatalln("创建collection出错:", err.Error())
		}
	}

	points := []*pb.PointStruct{}
	for _, _url := range urls {
		p, err := qdrant.BuildPointByUrl(_url)
		if err != nil {
			log.Fatalln("创建point出错:", err.Error())
		}
		points = append(points, p)
		fmt.Println(p.Id.String())
	}
	err = qdrant.FastQdrantClient.CreatePoints("kubernetes", points)
	if err != nil {
		log.Fatalln("批量创建point出错:", err.Error())
	}
}
