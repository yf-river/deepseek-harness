// ChainWeaver Env 数据接入。
//
// cwe 的 HTTP API 只提供 deploy / free / lease 查询这类操作接口，没有"列出全部环境"的读接口，
// 所以环境池状态直接读它的 state.json——那是 cwe 自己持久化的真源，和 CLI 的 pool_status 同源。
// 本文件只做只读解析，不写这个文件；释放环境走 cwe 的 HTTP 接口，见 robot 之外的 envFree。

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

// envStateFile 对应 cwe state.json 的顶层结构。只取门户需要的字段，
// 不解析 health 里的完整 Pod 明细——那部分体积大且对页面没用。
type envStateFile struct {
	Version      int              `json:"version"`
	Environments []envRecord      `json:"environments"`
	Leases       []envLeaseRecord `json:"leases"`
	Operations   []envOperation   `json:"operations"`
}

// envOperation 是一次环境操作（部署/释放）的执行记录。
type envOperation struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Status      string `json:"status"`
	Environment string `json:"environment"`
	Lease       string `json:"lease"`
	// Step 是阶段编号（数字），不是文字描述。
	Step        int    `json:"step"`
	Message     string `json:"message"`
	CreatedAt   string `json:"created_at"`
	StartedAt   string `json:"started_at"`
	FinishedAt  string `json:"finished_at"`
	// Error 是失败时的具体原因与建议动作，cwe 只在这里给出可读说明。
	Error *envOperationError `json:"error"`
}

// envOperationError 是失败原因。message 说明发生了什么，
// next_action 说明调用方可以怎么处理，两者都有才是完整信息。
type envOperationError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	NextAction string `json:"next_action"`
}

// envRecord 是一个环境的持久化状态。
type envRecord struct {
	Name             string            `json:"name"`
	Version          string            `json:"version"`
	Mode             string            `json:"mode"`
	Status           string            `json:"status"`
	Modules          []envModule       `json:"modules"`
	Ports            map[string]int    `json:"ports"`
	Database         string            `json:"database"`
	Namespaces       []string          `json:"namespaces"`
	OwnershipID      string            `json:"ownership_id"`
	LastReleasedAt   string            `json:"last_released_at"`
	RegisteredAt     string            `json:"registered_at"`
	UpdatedAt        string            `json:"updated_at"`
	Baseline         envBaseline       `json:"baseline"`
	Health           envHealth         `json:"health"`
}

// envModule 是一个环境里登记的业务模块。只取名称用于展示，部署仓库与 commit 不往页面送。
type envModule struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type envBaseline struct {
	Branch string `json:"branch"`
}

// envHealth 只取整体就绪与检查时间，Pod 级明细不往页面送。
type envHealth struct {
	CheckedAt string `json:"checked_at"`
	Ready     bool   `json:"ready"`
}

type envLeaseRecord struct {
	ID          string `json:"id"`
	Environment string `json:"environment"`
	Owner       string `json:"owner"`
	ExpiresAt   string `json:"expires_at"`
	DeployedAt  string `json:"deployed_at"`
}

// envView 是给前端的单条环境记录。
type envView struct {
	Name        string         `json:"name"`
	Version     string         `json:"version"`
	Mode        string         `json:"mode"`
	Status      string         `json:"status"`
	Modules     []string       `json:"modules"`
	// ModuleNamespaces 是模块名到命名空间的映射，便于看出模块落在哪个 namespace。
	ModuleNamespaces map[string]string `json:"moduleNamespaces"`
	// Namespaces 是该环境占用的全部 Kubernetes 命名空间。
	Namespaces []string `json:"namespaces"`
	Database    string         `json:"database"`
	Branch      string         `json:"branch"`
	Ready       bool           `json:"ready"`
	CheckedAt   string         `json:"checkedAt"`
	Ports       map[string]int `json:"ports"`
	// UIPort 是前端 NodePort，UIURL 由它和 EnvHost 拼出可直接打开的地址。
	UIPort int    `json:"uiPort"`
	UIURL  string `json:"uiUrl"`
	// Lease 为 nil 表示该环境当前没有被占用。
	Lease *envLeaseView `json:"lease"`
	UpdatedAt string    `json:"updatedAt"`
}

type envLeaseView struct {
	ID        string `json:"id"`
	Owner     string `json:"owner"`
	ExpiresAt string `json:"expiresAt"`
}

// envPool 是 /api/env 的返回体。
type envPool struct {
	Environments []envView `json:"environments"`
	Total        int       `json:"total"`
	Idle         int       `json:"idle"`
	Leased       int       `json:"leased"`
	Unhealthy    int       `json:"unhealthy"`
	// Busy 是正在部署或释放中的环境数，既不算空闲也不算已租用。
	Busy int `json:"busy"`
	// CheckedAt 是全池最近一次健康检查时间。
	CheckedAt string `json:"checkedAt"`
	// RecentOperations 是当前页的操作记录，最新的在前。
	RecentOperations []envOperation `json:"recentOperations"`
	// OperationsTotal 是操作记录总数，用于前端分页。
	OperationsTotal int `json:"operationsTotal"`
	// OperationsPage 与 OperationsPageSize 回显分页参数。
	OperationsPage     int `json:"operationsPage"`
	OperationsPageSize int `json:"operationsPageSize"`
	// Error 非空表示读取 state.json 失败，此时 Environments 为空。
	Error string `json:"error,omitempty"`
}

// loadEnvPool 通过 ssh 读取 cwe 的 state.json 并汇总成环境池视图。
//
// 为什么走 ssh：cwe 与门户不在同一台机器上（cwe 在 40.247），它又没有"列出环境"的
// HTTP 接口，只能读那份状态文件。走 ssh 读，不复制文件、不额外起同步任务，
// 读到的始终是 cwe 正在写的那一份。
func loadEnvPool(sshHost, envHost, statePath string, page, pageSize int) envPool {
	// ssh_host 为空表示 cwe 与本进程同机，直接读本地文件，不走 ssh。
	var raw []byte
	var err error
	if sshHost == "" {
		raw, err = os.ReadFile(statePath)
	} else {
		command := exec.Command("ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=6", sshHost, "cat "+statePath)
		raw, err = command.Output()
	}
	if err != nil {
		return envPool{Error: "读取 cwe 状态失败: " + err.Error()}
	}
	pool := parseEnvPool(raw, page, pageSize)
	// 前端链接由端口 + 宿主机地址拼出。cwe 的 state.json 只有 NodePort，
	// 不知道宿主机是谁，所以这部分在读取层补齐。
	for index := range pool.Environments {
		if pool.Environments[index].UIPort > 0 && envHost != "" {
			pool.Environments[index].UIURL = fmt.Sprintf("http://%s:%d/", envHost, pool.Environments[index].UIPort)
		}
	}
	return pool
}

// parseEnvPool 把 state.json 的原始字节解析成环境池视图。抽成独立函数便于单测。
func parseEnvPool(raw []byte, page, pageSize int) envPool {
	var state envStateFile
	if err := json.Unmarshal(raw, &state); err != nil {
		return envPool{Error: "解析 cwe 状态失败: " + err.Error()}
	}

	// 租约按环境名索引，环境记录本身只有一个 last_released_lease_id。
	leases := make(map[string]envLeaseRecord, len(state.Leases))
	for _, lease := range state.Leases {
		leases[lease.Environment] = lease
	}

	pool := envPool{Total: len(state.Environments), Environments: make([]envView, 0, len(state.Environments))}
	for _, record := range state.Environments {
		view := envView{
			Name:      record.Name,
			Version:   record.Version,
			Mode:      record.Mode,
			Status:    record.Status,
			Modules:          moduleNames(record.Modules),
			ModuleNamespaces: moduleNamespaces(record.Modules),
			Namespaces:       record.Namespaces,
			Database:  record.Database,
			Branch:    record.Baseline.Branch,
			Ready:     record.Health.Ready,
			CheckedAt: record.Health.CheckedAt,
			Ports:     record.Ports,
			UIPort:    record.Ports["ui"],
			UpdatedAt: record.UpdatedAt,
		}
		if lease, ok := leases[record.Name]; ok {
			view.Lease = &envLeaseView{ID: lease.ID, Owner: lease.Owner, ExpiresAt: lease.ExpiresAt}
		}

		// 只有真正空闲的才算可分配：leased 与 restoring 都占着环境，
		// 把 restoring 计成 idle 会让人误以为可以立刻用。
		// 顺序很重要：健康状态优先判定。不健康的环境即便 status 是 idle，
		// 也不能算成「可分派」——把它报成空闲会让人误以为可以立刻拿去用。
		// 判定顺序：不健康 > 有租约 > 处理中 > 空闲。
		// 有租约优先于 status：restoring 期间租约可能还没吊销，此时它仍然是"被占用"，
		// 报成处理中会让人误以为马上就能用。
		switch {
		case !record.Health.Ready:
			pool.Unhealthy++
		case view.Lease != nil:
			pool.Leased++
		case record.Status == "restoring", record.Status == "deploying", record.Status == "freeing":
			pool.Busy++
		default:
			pool.Idle++
		}
		if record.Health.CheckedAt > pool.CheckedAt {
			pool.CheckedAt = record.Health.CheckedAt
		}
		pool.Environments = append(pool.Environments, view)
	}

	// 操作记录按创建时间倒序分页。state.json 是追加写入，越靠后越新，
	// 所以从尾部往前取；分页也在这一层做，避免把全部历史都传给前端。
	pool.OperationsTotal = len(state.Operations)
	pool.OperationsPage = page
	pool.OperationsPageSize = pageSize
	page, pageSize = normalizePage(page, pageSize)
	pool.OperationsPage = page
	pool.OperationsPageSize = pageSize

	// 第 page 页对应「倒数第 (page-1)*size 到倒数 page*size 条」。
	skip := (page - 1) * pageSize
	pool.RecentOperations = make([]envOperation, 0, pageSize)
	for offset := skip; offset < skip+pageSize; offset++ {
		index := len(state.Operations) - 1 - offset
		if index < 0 {
			break
		}
		pool.RecentOperations = append(pool.RecentOperations, state.Operations[index])
	}
	return pool
}

// 分页常量：每页 100 条，是用户要求的固定口径。
const (
	defaultPageSize = 100
	maxPageSize     = 100
)

// normalizePage 把分页参数收进合法范围。页码从 1 开始，页大小固定上限 100。
func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 || pageSize > maxPageSize {
		pageSize = defaultPageSize
	}
	return page, pageSize
}

// moduleNamespaces 返回模块名到命名空间的映射。
// 环境里有多个模块时，能直接看出每个落在哪个 namespace。
func moduleNamespaces(modules []envModule) map[string]string {
	if len(modules) == 0 {
		return nil
	}
	out := make(map[string]string, len(modules))
	for _, module := range modules {
		if module.Name != "" {
			out[module.Name] = module.Namespace
		}
	}
	return out
}

// moduleNames 把模块记录压成名称数组。
func moduleNames(modules []envModule) []string {
	names := make([]string, 0, len(modules))
	for _, module := range modules {
		if module.Name != "" {
			names = append(names, module.Name)
		}
	}
	return names
}
