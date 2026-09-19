-- node_progress 增加 AI 综合评价缓存列（节点完成后后台异步生成并写入，报告接口直接读取，避免每次请求等待 LLM）
ALTER TABLE node_progress ADD COLUMN ai_evaluation_json jsonb;
