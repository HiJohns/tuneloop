-- #2543：回滚确认会话表（与 up 成对；该表此前不存在，无存量数据需要保留）。
DROP TABLE IF EXISTS confirmation_sessions;
