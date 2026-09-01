package tool

import "sort"

// Registry 按名字注册/查询 Tool。新增/替换 Tool = 实现接口 + Register，执行引擎不感知(可热插拔)。
var registry = map[string]Tool{}

func Register(t Tool) {
	registry[t.Name()] = t
}

func Get(name string) (Tool, bool) {
	t, ok := registry[name]
	return t, ok
}

func List() []Tool {
	out := make([]Tool, 0, len(registry))
	for _, t := range registry {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}
