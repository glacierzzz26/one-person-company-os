package runtime

import "sort"

// Registry 按名字注册/查询 Runtime。新增/替换 Runtime = 实现接口 + Register，执行引擎不感知。
var registry = map[string]Runtime{}

func Register(r Runtime) {
	registry[r.Name()] = r
}

func Get(name string) (Runtime, bool) {
	r, ok := registry[name]
	return r, ok
}

func List() []Runtime {
	out := make([]Runtime, 0, len(registry))
	for _, r := range registry {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}
