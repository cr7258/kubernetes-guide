package filter

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	pb "github.com/qdrant/go-client/qdrant"
	"github.com/sashabaranov/go-openai"
	"myembedding/pkg/myai"
	"regexp"
	"strconv"
	"strings"
)

type StackOverflowModel struct {
	Url      string
	Title    string
	Question string
	Answers  []string
	RawText  string // 原始文本
}

func NewStackOverflowModel(rawText string, url string) *StackOverflowModel {
	return &StackOverflowModel{RawText: rawText, Answers: []string{}, Url: url}
}

// 解析 实体
func (sm *StackOverflowModel) Parse() error {
	err := sm.parseTitle()
	if err != nil {
		return err
	}
	err = sm.parseQA()
	if err != nil {
		return err
	}
	return nil

}
func (sm *StackOverflowModel) parseTitle() error {
	pattern := "\\*+\\s*(.*)\\s*\\*+" // 用正则表达式匹配
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(sm.RawText)
	if len(matches) > 0 {
		sm.Title = matches[1]
		return nil
	}
	return errors.New("title not found")

}

// 防止问题太长，我们只需取出 最上面得有部分
func (sm *StackOverflowModel) filterQuestion(question string) string {
	re := regexp.MustCompile(`^\n+`) //过滤开头的空行
	question = re.ReplaceAllString(question, "")
	// 如果 文字下方超过2个空行则截取
	re = regexp.MustCompile(`\n{3,}[\s\S]*`)
	question = re.ReplaceAllString(question, "")
	return question
}

// 解析问题和答案
func (sm *StackOverflowModel) parseQA() error {
	// 把标题去掉
	text := regexp.MustCompile(`\*+\s*(.*)\s*\*+\n`).ReplaceAllString(sm.RawText, "")
	q := regexp.MustCompile(`([\s\S]*)\n(\d+\sAnswers\s\d+|\d+\sAnswer\s\d+|\s+)\s*Sorted\s*by:`).FindStringSubmatch(text)

	if len(q) >= 1 {
		sm.Question = sm.filterQuestion(q[1])

		re := regexp.MustCompile(`Sorted\s*by:\s*Reset\sto\sdefault([\s\S]*)`)
		match := re.FindStringSubmatch(text)

		if len(match) < 1 {
			return nil
		} else {
			text = match[1]
			re := regexp.MustCompile(`\n{3,}`) // 匹配两个或多个换行符

			paragraphs := re.Split(text, -1)

			for _, p := range paragraphs {
				p = strings.TrimSpace(p)
				if p == "" {
					continue
				}
				sm.Answers = append(sm.Answers, p)
			}
		}
		return nil
	} else {
		//如果么有答案 ，那整个文字就是问题。
		sm.Question = text
		sm.Answers = []string{}
	}
	return nil
}

func md5str(s string) string {
	h := md5.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

// 获取向量
func (sm *StackOverflowModel) GetVec() ([]float32, error) {
	c := myai.NewOpenAiClient()
	req := openai.EmbeddingRequest{
		Input: []string{sm.Title, sm.Question},
		Model: openai.AdaEmbeddingV2,
	}
	rsp, err := c.CreateEmbeddings(context.TODO(), req)
	if err != nil {
		return nil, err
	}
	return rsp.Data[0].Embedding, nil
}

func (sm *StackOverflowModel) toPayload() map[string]*pb.Value {
	ret := make(map[string]*pb.Value)
	ret["title"] = &pb.Value{Kind: &pb.Value_StringValue{StringValue: sm.Title}}
	ret["question"] = &pb.Value{Kind: &pb.Value_StringValue{StringValue: sm.Question}}
	listValue := &pb.Value_ListValue{
		ListValue: &pb.ListValue{
			Values: []*pb.Value{},
		},
	}
	for _, answer := range sm.Answers {
		listValue.ListValue.Values = append(listValue.ListValue.Values, &pb.Value{Kind: &pb.Value_StringValue{StringValue: answer}})
	}
	ret["answers"] = &pb.Value{Kind: listValue}

	return ret
}

// 使用模型构建出Point . 用来准备 插入collectoin

func (sm *StackOverflowModel) BuildPoint() *pb.PointStruct {
	point := &pb.PointStruct{}
	point.Id = &pb.PointId{
		PointIdOptions: &pb.PointId_Uuid{
			Uuid: md5str(sm.Url), // url 是唯一的 ，因此能保证 id的唯一性
		},
	}
	vec, err := sm.GetVec()
	if err != nil {
		return nil
	}
	// 把
	point.Vectors = &pb.Vectors{
		VectorsOptions: &pb.Vectors_Vector{
			Vector: &pb.Vector{
				Data: vec,
			},
		},
	}
	point.Payload = sm.toPayload()
	return point
}

// oneof代表 只要 匹配到其中一个
// all 代表 后面的元素都要有
// exact 代表 完全 匹配。 默认是  只要包含就算
var removes = [][]string{
	{"oneof", "Add a comment"},
	{"oneof", "gold badge", "silver badge", "bronze badge"},
	{"oneof", "Add a comment |"},
	{"oneof", "Improve this answer"},
	{"oneof", "Improve this question"},
	{"oneof|exact", "Follow"},
	{"oneof", "/users/", "/posts/", "/p/", "/a/", "/q/"},
	{"oneof", "Accept all cookies"},
	{"oneof|exact", "Customize settings"},
	{"oneof", "more comments"},
	{"oneof", "Highest score (default) Trending"},
}

type StackOverflowFilter struct {
	Removes [][]string //这些直接删除
	Stop    []string   // 遇到这个字符串，直接停止扫描
	Text    string
	Min     int // 最小长度。 如果小于这个长度也不算
	Begin   string
	Url     string // 问题的URL
}

// 创建过滤器
// 数据清洗 用于过滤掉一些不需要的内容
func NewStackOverflowFilter(text string) *StackOverflowFilter {

	f := &StackOverflowFilter{Text: text, Min: 2}
	f.Removes = removes
	f.Stop = []string{"Highly active question", "The Overflow Blog", "Your Answer"}
	f.Begin = "*****************************"
	return f
}

func (f *StackOverflowFilter) Filter() string {

	f.FilterRegex() //先用正则替换一批
	newList := []string{}
	strlist := strings.Split(f.Text, "\n")
	for _, str := range strlist {
		if len(newList) == 0 && strings.Index(str, f.Begin) == -1 {
			continue
		}
		if str == "" { //保留空行
			newList = append(newList, str)
			continue
		}
		str = strings.TrimSpace(str)
		if f.isRemove(str) {
			continue
		}
		if f.isStop(str) {
			break
		}
		if len(str) < f.Min {
			continue
		}
		newList = append(newList, str)
	}
	return strings.Join(newList, "\n")
}

// 是否停止
func (f *StackOverflowFilter) isStop(t string) bool {
	for _, s := range f.Stop {
		if strings.Index(t, s) > -1 {
			return true
		}
	}
	return false
}
func (f *StackOverflowFilter) isCond(config string, sign string) bool {
	if strings.Index(config, sign) > -1 {
		return true
	}
	return false
}
func (f *StackOverflowFilter) isDate(t string) bool {
	if strings.Index(t, "at") > -1 && strings.Index(t, ",") > -1 {
		return true
	}
	return false
}

// 一些特殊的判断
func (f *StackOverflowFilter) isRemoveExt(t string) bool {
	// 判断t 是否是日期字符串,如Jul 23, 2022 at 0:04
	if f.isDate(t) {
		return true
	}
	if strings.Index(t, "-") == 0 {
		return true
	}
	//判断t是否是数字
	if _, err := strconv.Atoi(t); err == nil {
		return true
	}
	// 匹配 ： 数字 Next
	if regexp.MustCompile("^\\d+\\s*Next$").MatchString(t) {
		return true
	}
	return false

}

// isRemove 是否删除
func (f *StackOverflowFilter) isRemove(t string) bool {
	for _, config := range f.Removes {
		if f.isCond(config[0], "oneof") {
			for _, v := range config[1:] {
				if f.isCond(config[0], "exact") {
					if t == v {
						return true
					}
				} else {
					if strings.Index(t, v) > -1 {
						return true
					}
				}
			}
		} else if f.isCond(config[0], "all") {
			isAll := true
			for _, v := range config[1:] {
				if f.isCond(config[0], "exact") {
					if t != v {
						isAll = false
						break
					}
				} else {
					if strings.Index(t, v) == -1 {
						isAll = false
						break
					}
				}

			}
			if isAll {
				return true
			}
		}
	}
	return f.isRemoveExt(t)
}

// 额外的再替换一些正则
func (f *StackOverflowFilter) FilterRegex() {

	// 替换 1
	pattern := "Ask\\s*Question[\\s\\S]*Viewed\\s*.*\\s*times\n"
	reg := regexp.MustCompile(pattern)
	// 替换文本
	f.Text = reg.ReplaceAllString(f.Text, "")

	url := ""
	// 取出URL
	pattern = "\\*{10,}\\s*.*\\(\\s*/questions(.*?)\\s*\\)\\s*\\*+"
	matches := regexp.MustCompile(pattern).FindStringSubmatch(f.Text)
	if len(matches) > 1 {
		url = matches[1]
		f.Url = url
	}

	// 替换 2
	pattern = "\\(\\s*/questions.*?\\)"
	f.Text = regexp.MustCompile(pattern).ReplaceAllString(f.Text, "")

	// 替换掉 相关帖子
	pattern = "Related\\s*questions[\\s\\S]*\\n{2,}"
	f.Text = regexp.MustCompile(pattern).ReplaceAllString(f.Text, "")
}
