## 数据库问题修复

问题：后端启动失败，提示 properties 表已存在。

修复：使迁移文件幂等化，添加 IF NOT EXISTS 避免重复创建错误。

Commit: 853f3947

解决步骤：

方案 1：重置数据库
  pkill -f "go run main.go"
  cd backend && go run main.go --bootstrap

方案 2：重启服务
  make run

验证成功标志：Database bootstrap completed successfully

Model: moonshotai-cn/kimi-k2-thinking