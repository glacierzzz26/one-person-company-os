// Package schedule 提供流水线 schedule 字段的 cron 五段解析与下一命中计算(Phase 10.2)。
// 纯 Go 无外部依赖(单二进制零 CGO 红线);子集与语义见 Parse 注释。
package schedule

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Cron 是解析后的一条 cron 计划(5 段数字;不携带秒/年)。
// 内部以位图存每字段可取值;dayAllowed 采用 cron 标准 dom/dow 语义。
type Cron struct {
	minute field
	hour   field
	dom    field
	month  field
	dow    field
}

// field 一段取值位图(0-59 内以 uint64 位表达;all=true 表示该段不限)。
type field struct {
	all  bool
	bits uint64
}

func (f field) has(v int) bool {
	if f.all {
		return true
	}
	return f.bits&(1<<uint(v)) != 0
}

// fieldRange 五段的合法取值域(含端)。dow 用 raw 7(别名周日)校验;落位图时 v%7。
type fieldRange struct{ lo, hi int }

var fieldRanges = map[string]fieldRange{
	"minute": {0, 59}, "hour": {0, 23}, "dom": {1, 31},
	"month": {1, 12}, "dow": {0, 7}, // dow raw 0-7(7 = 周日别名;mapToBit %7 → 位 0)
}

// Parse 解析一条 cron 计划。`""`/`off` → scheduled=false(不调度,合法);
// 否则须为 5 段空白分隔 `分 时 日 月 周`,段值域:分0-59 时0-23 日1-31 月1-12 周0-6(0=周日;7 亦按周日)。
// 每段支持:`*` | `n` | `a-b` | `a-b/n` | `*/n` | 逗号并集。名字(MON/JAN)、`?`、`L/W/#`、秒/年、@别名 → 报错。
// dom 与 dow:两者都受限(非 *)时按 cron 标准 OR(任一命中即匹配);否则仅受限者约束。
func Parse(s string) (c Cron, scheduled bool, err error) {
	trim := strings.TrimSpace(s)
	if trim == "" || strings.EqualFold(trim, "off") {
		return Cron{}, false, nil
	}
	parts := strings.Fields(trim)
	if len(parts) != 5 {
		return Cron{}, false, fmt.Errorf("cron must have 5 fields (minute hour day-of-month month day-of-week), got %d", len(parts))
	}
	names := []string{"minute", "hour", "dom", "month", "dow"}
	fields := make([]field, 5)
	for i, raw := range parts {
		f, ferr := parseField(names[i], raw)
		if ferr != nil {
			return Cron{}, false, ferr
		}
		fields[i] = f
	}
	return Cron{minute: fields[0], hour: fields[1], dom: fields[2], month: fields[3], dow: fields[4]}, true, nil
}

// parseField 解析单个 cron 段为取值集合(逗号并集;含 * / 范围 / 步进)。
func parseField(name, raw string) (field, error) {
	parts := strings.Split(raw, ",")
	out := field{all: false, bits: 0}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			return field{}, fmt.Errorf("cron %s: empty list item in %q", name, raw)
		}
		f, err := parseUnit(name, p)
		if err != nil {
			return field{}, err
		}
		if f.all {
			out = field{all: true}
			return out, nil // 并集含 * → 全取,短路
		}
		out.bits |= f.bits
	}
	return out, nil
}

// parseUnit 解析单一项:见 Parse 段语法。
// dow 用 %7 归一(7 = 周日别名并入位 0);范围/步进/单值同走 mapToBit。
func parseUnit(name, p string) (field, error) {
	r := fieldRanges[name]
	mapToBit := func(v int) int {
		if name == "dow" {
			return v % 7
		}
		return v
	}
	// 取步进后缀 "/n"。
	step := 1
	if i := strings.IndexByte(p, '/'); i >= 0 {
		sv := p[i+1:]
		p = p[:i]
		if sv == "" {
			return field{}, fmt.Errorf("cron %s: empty step in %q", name, p+"/")
		}
		n, err := strconv.Atoi(sv)
		if err != nil || n < 1 {
			return field{}, fmt.Errorf("cron %s: invalid step %q (positive integer)", name, sv)
		}
		step = n
	}
	if p == "*" {
		// */n:从 lo 起每 n 个取值;* = 全取。
		if step > 1 {
			return steppedRange(r.lo, r.hi, step, mapToBit), nil
		}
		return field{all: true}, nil
	}
	// 单值或 a-b 范围(可带 /n)。
	if i := strings.IndexByte(p, '-'); i >= 0 {
		aStr, bStr := strings.TrimSpace(p[:i]), strings.TrimSpace(p[i+1:])
		a, err := strconv.Atoi(aStr)
		if err != nil {
			return field{}, fmt.Errorf("cron %s: bad range start %q", name, aStr)
		}
		b, err := strconv.Atoi(bStr)
		if err != nil {
			return field{}, fmt.Errorf("cron %s: bad range end %q", name, bStr)
		}
		if a < r.lo || b > r.hi || a > b {
			return field{}, fmt.Errorf("cron %s: range %d-%d out of %d..%d", name, a, b, r.lo, r.hi)
		}
		return steppedRange(a, b, step, mapToBit), nil
	}
	v, err := strconv.Atoi(p)
	if err != nil {
		return field{}, fmt.Errorf("cron %s: cannot parse %q (5-field numeric cron; e.g. 0 9 * * * = daily 09:00)", name, p)
	}
	if v < r.lo || v > r.hi {
		return field{}, fmt.Errorf("cron %s: value %d out of %d..%d", name, v, r.lo, r.hi)
	}
	return field{bits: 1 << uint(mapToBit(v))}, nil
}

// steppedRange 从 lo 起每 step 个取值到 hi(含 lo;步进不越 hi);取值经 mapToBit 落位。
func steppedRange(lo, hi, step int, mapToBit func(int) int) field {
	f := field{bits: 0}
	for v := lo; v <= hi; v += step {
		f.bits |= 1 << uint(mapToBit(v))
	}
	return f
}

// dayAllowed 判定某日是否命中月/日/周约束(cron 标准:dom 与 dow 都受限 → OR)。
func (c Cron) dayAllowed(d time.Time) bool {
	dom, dow, month := d.Day(), int(d.Weekday()), int(d.Month())
	domOK := c.dom.has(dom)
	dowOK := c.dow.has(dow)
	monthOK := c.month.has(month)
	if !monthOK {
		return false
	}
	domRestricted := !c.dom.all
	dowRestricted := !c.dow.all
	switch {
	case domRestricted && dowRestricted:
		return domOK || dowOK
	case domRestricted:
		return domOK
	case dowRestricted:
		return dowOK
	default:
		return true
	}
}

// hasInWindow 判定 (h,m) 是否命中 时/分 约束。
func (c Cron) timeAllowed(h, m int) bool {
	return c.hour.has(h) && c.minute.has(m)
}

// Next 返回严格晚于 now 的下一次命中(本地时区=now.Location);合法计划通常 5 年内必达,
// 极端永不命中(如 `0 0 31 2 *` 2月31日)返回零值 time.Time,调用方按无计划处理。
func (c Cron) Next(now time.Time) time.Time {
	loc := now.Location()
	// 从 now 起逐日扫描(至多 6 年,覆盖闰年 2/29);命中日在当日自 start 时刻逐分找首个 (h,m)。
	for day := 0; day <= 366*6; day++ {
		d := now.AddDate(0, 0, day)
		if !c.dayAllowed(d) {
			continue
		}
		startH, startM := 0, 0
		if day == 0 {
			// 严格晚于 now:从 now+1 分钟起扫。
			base := now.Add(time.Minute)
			startH, startM = base.Hour(), base.Minute()
		}
		for h := startH; h <= 23; h++ {
			if !c.hour.has(h) {
				continue
			}
			mLo := 0
			if h == startH {
				mLo = startM
			}
			for m := mLo; m <= 59; m++ {
				if c.minute.has(m) {
					return time.Date(d.Year(), d.Month(), d.Day(), h, m, 0, 0, loc)
				}
			}
		}
	}
	return time.Time{}
}

// String 输出便于审计/日志的原文等价形式(仅位图还原,非回写原串;供 detail 用)。
func (c Cron) String() string {
	return fmt.Sprintf("min=%v hour=%v dom=%v mon=%v dow=%v",
		c.minute.all, c.hour.all, c.dom.all, c.month.all, c.dow.all)
}
