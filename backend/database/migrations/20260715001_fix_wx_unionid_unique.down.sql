-- Add back the original unique constraint
ALTER TABLE users ADD CONSTRAINT users_wx_unionid_key UNIQUE (wx_unionid);
