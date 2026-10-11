package common

// Milvus 数据源与集合的默认名称。
const (
	MilvusDBName         = "agent" // Milvus 数据源名称
	MilvusCollectionName = "biz"   // Milvus 集合名称
)

// FileDir 是知识库文档存放目录，内容属于运行期数据，默认不纳入版本控制。
// 例外：随仓库提供 docs/告警处理手册.md 作为初始化知识库文档。
var FileDir = "./docs/"
