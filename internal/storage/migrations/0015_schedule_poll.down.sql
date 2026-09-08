-- Phase 10.2 回滚:app_setting.schedule_poll_sec 列移除(只增不改,直接删列)。
ALTER TABLE app_setting DROP COLUMN schedule_poll_sec;
