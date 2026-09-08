-- Phase 10.2 独立调度总开关(契约 ops-patrol-schedule.md §3.7):app_setting.schedule_poll_sec。
--   0(缺省)= 关:ScheduleLoop 不启动,流水线 schedule 只存不触发;>0 = 秒级轮询间隔,到点触发一次 run。
--   与 queue_work 正交(到点只建单;任务执行仍受「后台执行」总开关)。只增不改。
ALTER TABLE app_setting ADD COLUMN schedule_poll_sec INTEGER NOT NULL DEFAULT 0;
