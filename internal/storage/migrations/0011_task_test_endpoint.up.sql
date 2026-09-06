-- Phase 8.4: task test 判读槽(方向 §三 test=standard 与 review=frontier 分槽)。
-- test_endpoint_id 与 writer/reviewer 正交:test 判读默认落 standard 档端点(reviewer 默认 frontier),
-- engEndpointFor test → test_endpoint_id → reviewer_endpoint_id → writer_endpoint_id 渐进回退。

ALTER TABLE task ADD COLUMN test_endpoint_id TEXT REFERENCES endpoint(id);
