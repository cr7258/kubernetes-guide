package httphelper

import (
	"io/ioutil"
	"net/http"
	"time"
)

func RequestUrl(inputurl string) (string, error) {
	// 创建HTTP客户端
	client := &http.Client{
		Timeout:   10 * time.Second, // 设置超时时间
		Transport: &http.Transport{},
	}
	// 构造请求,添加User-Agent等headers
	req, err := http.NewRequest("GET", inputurl, nil)
	if err != nil {
		return "", err
	}
	req.Header.Add("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.114 Safari/537.36")

	// 发送请求并获取响应
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	// 关闭响应体
	defer resp.Body.Close()

	// 读取响应内容
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil

}
