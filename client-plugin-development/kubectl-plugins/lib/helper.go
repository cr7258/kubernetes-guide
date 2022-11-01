package lib

import (
	"fmt"
	"github.com/olekukonko/tablewriter"
)

func Map2String(m map[string]string) (ret string) {
	for k, v := range m {
		ret += fmt.Sprintf("%s=%s\n", k, v)
	}
	return
}

func InitHeader(table *tablewriter.Table) []string {
	commonHeaders := []string{"名称", "命名空间", "IP", "状态"}
	if ShowLabels {
		commonHeaders = append(commonHeaders, "标签")
	}
	return commonHeaders
}
