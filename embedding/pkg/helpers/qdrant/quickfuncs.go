package qdrant

import (
	"github.com/google/uuid"
	"github.com/jaytaylor/html2text"
	pb "github.com/qdrant/go-client/qdrant"
	"myembedding/pkg/helpers/filter"
	"myembedding/pkg/helpers/httphelper"
	"os"
)

// 这里包含 一些快速函数。 省的 还要创建 client，然后再调用，太麻烦了
var FastQdrantClient *QdrantClient

func init() {
	FastQdrantClient = NewQdrantClient()
}
func FastAddPoints(cname string, vecs []float32, payload map[string]string) error {
	uid, _ := uuid.NewUUID()
	return FastQdrantClient.CreatePoint(uid.String(), cname, vecs, payload)
}

type Collection string

func (c Collection) Create(size uint64) error {
	return FastQdrantClient.CreateCollection(string(c), size)
}

func HasCollection(collectionName string) (bool, error) {
	collection, err := FastQdrantClient.GetCollection(collectionName)
	if err != nil {
		return false, err
	}
	if collection != nil {
		return true, nil
	}
	return false, nil
}

func (c Collection) Delete() error {
	return FastQdrantClient.DeleteCollection(string(c))
}

// 根据Url 一条龙 构建Point
func BuildPointByUrl(u string) (*pb.PointStruct, error) {
	s, err := httphelper.RequestUrl(u)
	if err != nil {
		return nil, err
	}

	text, err := html2text.FromString(s, html2text.Options{
		PrettyTables: true,
	})
	if err != nil {
		return nil, err
	}

	ft := filter.NewStackOverflowFilter(text)
	text = ft.Filter() //特征过滤（含少量 正则过滤)
	f, _ := os.OpenFile("test.txt", os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0666)
	f.WriteString(text)
	defer f.Close()

	// filter 会产生url 出来
	m := filter.NewStackOverflowModel(text, ft.Url)
	err = m.Parse() //解析模型（正则获取+少量文本切割)
	if err != nil {
		return nil, err
	}

	return m.BuildPoint(), nil

}
