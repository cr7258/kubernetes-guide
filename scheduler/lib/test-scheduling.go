package lib

import (
	"context"
	"fmt"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	frameworkruntime "k8s.io/kubernetes/pkg/scheduler/framework/runtime"
)

const TestSchedulingName = "test-scheduling"

type TestScheduling struct {
	fact informers.SharedInformerFactory
	args *Args
}

type Args struct {
	MaxPods int `json:"maxPods,omitempty"`
}

func (s *TestScheduling) AddPod(ctx context.Context, state *framework.CycleState, podToSchedule *v1.Pod, podInfoToAdd *framework.PodInfo, nodeInfo *framework.NodeInfo) *framework.Status {
	return nil
}

func (s *TestScheduling) RemovePod(ctx context.Context, state *framework.CycleState, podToSchedule *v1.Pod, podInfoToRemove *framework.PodInfo, nodeInfo *framework.NodeInfo) *framework.Status {
	return nil
}

//业务方法
func (s *TestScheduling) PreFilter(ctx context.Context, state *framework.CycleState, p *v1.Pod) *framework.Status {
	klog.Info("预过滤")
	pods, err := s.fact.Core().V1().Pods().Lister().Pods(p.Namespace).List(labels.Everything())
	if err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}
	if s.args.MaxPods > 0 && len(pods) > s.args.MaxPods {
		return framework.NewStatus(framework.Unschedulable, fmt.Sprintf("POD数量超过了，不给调度了，最多只能是 %d 个", s.args.MaxPods))
	}
	return framework.NewStatus(framework.Success)
}

func (s *TestScheduling) PreFilterExtensions() framework.PreFilterExtensions {
	return s
}

func (s *TestScheduling) Filter(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	klog.Info("开始过滤")
	// 节点标签是 scheduling=false 的不可调度
	for k, v := range nodeInfo.Node().Labels {
		if k == "scheduling" && v != "true" {
			return framework.NewStatus(framework.Unschedulable, "这个节点不可调度")
		}
	}
	return framework.NewStatus(framework.Success)
}

func (s *TestScheduling) Score(ctx context.Context, state *framework.CycleState, p *v1.Pod, nodeName string) (int64, *framework.Status) {
	if nodeName == "cluster02-2" {
		return 20, framework.NewStatus(framework.Success)
	}
	return 10, framework.NewStatus(framework.Success)
}
func (s *TestScheduling) NormalizeScore(ctx context.Context, state *framework.CycleState, p *v1.Pod, scores framework.NodeScoreList) *framework.Status {
	var min, max int64 = 0, 0
	//先求出 最小分数和最大分数
	for _, score := range scores {
		if score.Score < min {
			min = score.Score
		}
		if score.Score > max {
			max = score.Score
		}
	}
	if max == min {
		min = min - 1
	}

	for i, score := range scores {
		scores[i].Score = (score.Score - min) * framework.MaxNodeScore / (max - min)
		klog.Infof("节点: %v, Score: %v   Pod:  %v", scores[i].Name, scores[i].Score, p.GetName())
	}
	return framework.NewStatus(framework.Success, "")

}

func (s *TestScheduling) ScoreExtensions() framework.ScoreExtensions {
	return s
}

func (*TestScheduling) Name() string {
	return TestSchedulingName
}

var _ framework.PreFilterPlugin = &TestScheduling{}
var _ framework.FilterPlugin = &TestScheduling{}
var _ framework.ScorePlugin = &TestScheduling{}

func NewTestScheduling(configuration runtime.Object, f framework.Handle) (framework.Plugin, error) {
	args := &Args{}
	if err := frameworkruntime.DecodeInto(configuration, args); err != nil {
		return nil, err
	}
	return &TestScheduling{
		fact: f.SharedInformerFactory(),
		args: args,
	}, nil
}
