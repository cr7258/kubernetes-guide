package datafetcher

import (
	"bufio"
	"fmt"
	"log"
	"myembedding/pkg/helpers/httphelper"
	"os"
	"regexp"
)

const K8sUrl = "https://stackoverflow.com/questions/tagged/kubernetes?tab=active&page=%d&pagesize=50"

func GetQuestions(page int) []string {
	url := fmt.Sprintf(K8sUrl, page)
	log.Println("开始抓取:", url)
	ret := []string{}
	body, err := httphelper.RequestUrl(url)
	if err != nil {
		fmt.Println("请求出错:", err.Error(), "page=", page)
		return ret
	}
	matches := regexp.MustCompile(`<a\shref="/questions/(\d+)/(.*?)"\sclass="s-link"`).
		FindAllStringSubmatch(body, -1)
	for _, m := range matches {
		if len(m) > 1 {
			ret = append(ret, fmt.Sprintf("/questions/%s/%s\n", m[1], m[2]))

		}
	}
	return ret
}

func SaveLinks(file string, data []string) {
	// 打开文件进行写入
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer f.Close()

	// 使用bufio包提供的写入功能
	writer := bufio.NewWriter(f)
	for _, str := range data {
		fmt.Fprintln(writer, str)
	}
	writer.Flush()

	fmt.Println("写入完成")
}
