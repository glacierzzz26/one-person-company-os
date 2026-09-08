package project

// 项目 = 整项目一个 git 仓库的目录容器(Phase 10 方向 D7 案A:layout A)。
// root_path 为绝对路径:须指向已存在 git 仓库(有 .git),或允许 OS mkdir + git init(create 时做)。
// 删除项目仅断引用 + 删流水线元数据,永不触碰磁盘目录(受控 DELETE)。

type Project struct {
	ID          string `json:"id"`
	CompanyID   string `json:"company_id"`
	Name        string `json:"name"`
	RootPath    string `json:"root_path"` // 绝对路径;整项目 git 仓库根
	Description string `json:"description"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}
