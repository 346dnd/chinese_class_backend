-- 中华文化教学后端 初始化 schema（基于 默认模块.openapi.json 全新设计）
-- 主键策略：bigserial 内部主键 + varchar(64) code 对外业务编码
-- 节点 ID 全局唯一：node_progress / node_submissions 仅以 (node_id, user_id) 标识
-- 历史保留：node_submissions 每次 submit 都 INSERT 新行，保留全部答题记录（含错误、重试、get-answer）
-- 报告字段实时聚合：studyDuration / points / likeCount 等不持久化，查询时聚合计算

-- ============== 扩展 ==============
CREATE EXTENSION IF NOT EXISTS "pgcrypto";   -- gen_random_uuid 用于生成 code

-- ============== 枚举类型 ==============
CREATE TYPE user_role              AS ENUM ('admin', 'teacher', 'student');
CREATE TYPE scene_type             AS ENUM ('inclass', 'afterclass');
CREATE TYPE node_type              AS ENUM (
    'write-thoughts', 'initial-insight', 'cultural-style',
    'cultural-style-quick-select', 'cultural-style-drag-sort',
    'zhaozhouqiao', 'wenmingzhongwai', 'heritage-cultural',
    'creation-workshop', 'folder'
);
CREATE TYPE creation_task_type     AS ENUM ('generation', 'poemscripts', 'share', 'express');
CREATE TYPE creation_status        AS ENUM ('pending', 'processing', 'successful', 'published');
CREATE TYPE chat_finish_reason     AS ENUM ('stop', 'length', 'tool_calls');
CREATE TYPE audio_trans_status     AS ENUM ('processing', 'finish', 'fail');
CREATE TYPE ai_message_role        AS ENUM ('user', 'assistant', 'system');
CREATE TYPE resource_media_type    AS ENUM ('image', 'audio', 'video');
CREATE TYPE activity_status        AS ENUM ('NotStarted', 'InProgress', 'Finished');

-- ============== users 用户 ==============
CREATE TABLE users (
    id              bigserial      PRIMARY KEY,
    code            varchar(64)    NOT NULL,
    username        varchar(64)    NOT NULL,
    password_hash   varchar(255)   NOT NULL,
    role            user_role      NOT NULL DEFAULT 'student',
    real_name       varchar(64),
    avatar_url      text,
    gender          varchar(16),
    city            varchar(128),
    grade_name      varchar(64),
    class_name      varchar(128),
    status          smallint       NOT NULL DEFAULT 1,   -- 1=启用 0=禁用
    last_login_at   timestamptz,
    created_at      timestamptz    NOT NULL DEFAULT now(),
    updated_at      timestamptz    NOT NULL DEFAULT now(),
    deleted_at      timestamptz
);
CREATE UNIQUE INDEX users_code_uniq     ON users (code)     WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX users_username_uniq ON users (username) WHERE deleted_at IS NULL;
CREATE INDEX        users_role_idx      ON users (role);

-- ============== classes 班级 ==============
CREATE TABLE classes (
    id            bigserial     PRIMARY KEY,
    code          varchar(64)   NOT NULL,
    name          varchar(128)  NOT NULL,
    description   text,
    teacher_id    bigint        REFERENCES users(id),
    scene         scene_type    NOT NULL DEFAULT 'inclass',
    start_at      timestamptz,
    end_at        timestamptz,
    status        smallint      NOT NULL DEFAULT 1,
    created_at    timestamptz   NOT NULL DEFAULT now(),
    updated_at    timestamptz   NOT NULL DEFAULT now(),
    deleted_at    timestamptz
);
CREATE UNIQUE INDEX classes_code_uniq   ON classes (code) WHERE deleted_at IS NULL;
CREATE INDEX        classes_teacher_idx ON classes (teacher_id);

-- ============== class_members 班级成员 ==============
CREATE TABLE class_members (
    id          bigserial   PRIMARY KEY,
    class_id    bigint      NOT NULL REFERENCES classes(id) ON DELETE CASCADE,
    user_id     bigint      NOT NULL REFERENCES users(id),
    role        user_role   NOT NULL DEFAULT 'student',
    joined_at   timestamptz NOT NULL DEFAULT now(),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (class_id, user_id)
);
CREATE INDEX class_members_user_idx ON class_members (user_id);

-- ============== class_nodes 课程节点（树形） ==============
-- node_id 全局唯一：同一节点可被多个班级共享（通过 class_id 标识所属课堂）
CREATE TABLE class_nodes (
    id           bigserial    PRIMARY KEY,
    code         varchar(64),
    class_id     bigint       NOT NULL REFERENCES classes(id) ON DELETE CASCADE,
    parent_id    bigint       REFERENCES class_nodes(id) ON DELETE CASCADE,
    node_type    node_type    NOT NULL,
    title        varchar(255) NOT NULL,
    description  text,
    sort_order   int          NOT NULL DEFAULT 0,
    has_children boolean      NOT NULL DEFAULT false,
    scene        scene_type,
    created_at   timestamptz  NOT NULL DEFAULT now(),
    updated_at   timestamptz  NOT NULL DEFAULT now(),
    deleted_at   timestamptz
);
CREATE UNIQUE INDEX class_nodes_code_uniq   ON class_nodes (code) WHERE deleted_at IS NULL AND code IS NOT NULL;
CREATE INDEX        class_nodes_class_idx   ON class_nodes (class_id);
CREATE INDEX        class_nodes_parent_idx  ON class_nodes (parent_id);
CREATE INDEX        class_nodes_sort_idx    ON class_nodes (class_id, parent_id, sort_order);

-- ============== node_contents 节点 params 配置（1:1） ==============
-- 存储 *Params schema 的静态配置（introVideo / title / description / matrix 等）
CREATE TABLE node_contents (
    id            bigserial   PRIMARY KEY,
    node_id       bigint      NOT NULL UNIQUE REFERENCES class_nodes(id) ON DELETE CASCADE,
    version       int         NOT NULL DEFAULT 1,
    params_json   jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX node_contents_params_gin ON node_contents USING gin (params_json jsonb_path_ops);

-- ============== node_progress 学习进度缓存（1:1 per node+user） ==============
-- 仅作 state 接口缓存，不存储历史；历史在 node_submissions
CREATE TABLE node_progress (
    id              bigserial   PRIMARY KEY,
    node_id         bigint      NOT NULL REFERENCES class_nodes(id) ON DELETE CASCADE,
    user_id         bigint      NOT NULL REFERENCES users(id),
    class_id        bigint      NOT NULL REFERENCES classes(id) ON DELETE CASCADE,
    completed       boolean     NOT NULL DEFAULT false,
    attempt_count   int         NOT NULL DEFAULT 0,
    error_count     int         NOT NULL DEFAULT 0,
    revealed        boolean     NOT NULL DEFAULT false,
    draft_json      jsonb       NOT NULL DEFAULT '{}'::jsonb,
    last_submit_id  bigint,
    duration        int         NOT NULL DEFAULT 0,   -- 累计用时（秒），由 submissions 同步
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (node_id, user_id)
);
CREATE INDEX node_progress_user_idx   ON node_progress (user_id, class_id);
CREATE INDEX node_progress_class_idx  ON node_progress (class_id);

-- ============== node_submissions 节点提交明细（保留全部历史） ==============
-- 每次 submit 都 INSERT 新行，含错误、重试、get-answer
-- 对应 *SubmitResult / *SumbitResp schema
CREATE TABLE node_submissions (
    id                 bigserial    PRIMARY KEY,
    code               varchar(64)  NOT NULL,                  -- 对外 submitId
    class_id           bigint       NOT NULL REFERENCES classes(id) ON DELETE CASCADE,
    node_id            bigint       NOT NULL REFERENCES class_nodes(id) ON DELETE CASCADE,
    user_id            bigint       NOT NULL REFERENCES users(id),
    node_type          node_type    NOT NULL,
    question_id        varchar(64),                            -- write-thoughts / initial-insight / zhaozhouqiao 等
    submit_type        varchar(16)  NOT NULL DEFAULT 'submit',  -- submit / get-answer
    payload_json       jsonb        NOT NULL DEFAULT '{}'::jsonb,  -- 学生提交的原始内容
    result_json        jsonb,                                   -- AI 判定/反馈结构化结果
    is_processing      boolean      NOT NULL DEFAULT false,
    is_passed          boolean,
    is_completed       boolean,
    error_count        int          NOT NULL DEFAULT 0,
    revealed           boolean      NOT NULL DEFAULT false,
    feedback           text,                                    -- AI 文本反馈
    reference_answer   text,                                    -- 参考答案
    audio_resource_id  bigint,                                   -- 朗读题录音 resource
    duration           int,                                     -- 用时（秒）
    points_earned      int          NOT NULL DEFAULT 0,
    submitted_at       timestamptz  NOT NULL DEFAULT now(),
    created_at         timestamptz  NOT NULL DEFAULT now(),
    updated_at         timestamptz  NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX node_submissions_code_uniq ON node_submissions (code);
CREATE INDEX node_submissions_user_idx         ON node_submissions (user_id, class_id, node_id);
CREATE INDEX node_submissions_node_idx         ON node_submissions (class_id, node_id, submitted_at DESC);
CREATE INDEX node_submissions_payload_gin      ON node_submissions USING gin (payload_json);

-- ============== creation_tasks 创作工坊任务定义 ==============
-- taskId 字符串对应 creation-workshop/{taskId}
CREATE TABLE creation_tasks (
    id           bigserial         PRIMARY KEY,
    code         varchar(64)       NOT NULL,                -- 对外 taskId
    node_id      bigint            NOT NULL REFERENCES class_nodes(id) ON DELETE CASCADE,
    task_type    creation_task_type NOT NULL,
    title        varchar(255),
    config_json  jsonb,                                     -- GetParams 配置
    created_at   timestamptz       NOT NULL DEFAULT now(),
    updated_at   timestamptz       NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX creation_tasks_code_uniq ON creation_tasks (code);
CREATE INDEX        creation_tasks_node_idx  ON creation_tasks (node_id);

-- ============== creation_submissions 创作工坊提交 ==============
-- 对应 GetState.creationLogs / GetResult / AIevaluation / PublishResult
CREATE TABLE creation_submissions (
    id                   bigserial         PRIMARY KEY,
    code                 varchar(64)       NOT NULL,                  -- 对外 submitId
    task_id              bigint            NOT NULL REFERENCES creation_tasks(id) ON DELETE CASCADE,
    node_id              bigint            NOT NULL REFERENCES class_nodes(id) ON DELETE CASCADE,
    user_id              bigint            NOT NULL REFERENCES users(id),
    class_id             bigint            NOT NULL REFERENCES classes(id) ON DELETE CASCADE,
    task_type            creation_task_type NOT NULL,
    text_content         text,                                        -- 用户输入文字
    content_json         jsonb            NOT NULL DEFAULT '{}'::jsonb, -- 结构化输入
    ai_image_resource_id bigint,                                       -- AI 生成的图片 resource
    ai_feedback          text,                                        -- AI 反馈文本
    ai_revised_content   text,                                        -- AI 修改后的文本
    ai_evaluation_json   jsonb,                                        -- AIevaluation 结构化结果
    status               creation_status  NOT NULL DEFAULT 'pending',
    is_processing        boolean          NOT NULL DEFAULT false,
    is_successful        boolean          NOT NULL DEFAULT false,
    is_completed         boolean          NOT NULL DEFAULT false,
    is_published         boolean          NOT NULL DEFAULT false,
    duration             int              NOT NULL DEFAULT 0,
    points_earned        int              NOT NULL DEFAULT 0,
    submitted_at         timestamptz      NOT NULL DEFAULT now(),
    published_at         timestamptz,
    created_at           timestamptz      NOT NULL DEFAULT now(),
    updated_at           timestamptz      NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX creation_submissions_code_uniq ON creation_submissions (code);
CREATE INDEX creation_submissions_user_idx         ON creation_submissions (user_id, class_id, node_id);
CREATE INDEX creation_submissions_task_idx         ON creation_submissions (task_id, submitted_at DESC);

-- ============== resources 文件资源 ==============
CREATE TABLE resources (
    id                bigserial      PRIMARY KEY,
    resource_id       varchar(64)    NOT NULL,
    media_type        resource_media_type NOT NULL,
    file_name         varchar(255)   NOT NULL,
    file_url          text,
    resource_suffix   varchar(16)    NOT NULL,
    resource_size     bigint         NOT NULL DEFAULT 0,
    resource_info     text,
    resource_source   varchar(64),
    upload_oss        boolean        NOT NULL DEFAULT true,
    image_compress    boolean        NOT NULL DEFAULT false,
    video_compress    boolean        NOT NULL DEFAULT false,
    owner_id          bigint         REFERENCES users(id),
    created_at        timestamptz    NOT NULL DEFAULT now(),
    updated_at        timestamptz    NOT NULL DEFAULT now(),
    deleted_at        timestamptz
);
CREATE UNIQUE INDEX resources_resource_id_uniq ON resources (resource_id);
CREATE INDEX        resources_owner_idx        ON resources (owner_id);
CREATE INDEX        resources_media_idx        ON resources (media_type);
CREATE INDEX        resources_time_idx         ON resources (created_at DESC);

-- ============== audio_transcriptions 语音转写任务 ==============
CREATE TABLE audio_transcriptions (
    id           bigserial          PRIMARY KEY,
    resource_id  bigint             NOT NULL UNIQUE REFERENCES resources(id) ON DELETE CASCADE,
    status       audio_trans_status NOT NULL DEFAULT 'processing',
    text         text,
    error_msg    text,
    created_at   timestamptz        NOT NULL DEFAULT now(),
    updated_at   timestamptz        NOT NULL DEFAULT now()
);
CREATE INDEX audio_transcriptions_pending_idx ON audio_transcriptions (resource_id) WHERE status = 'processing';

-- ============== ai_sessions AI 会话 ==============
CREATE TABLE ai_sessions (
    id               bigserial    PRIMARY KEY,
    code             varchar(64)  NOT NULL,
    user_id          bigint       REFERENCES users(id),
    class_id         bigint       REFERENCES classes(id) ON DELETE SET NULL,
    node_id          bigint       REFERENCES class_nodes(id) ON DELETE SET NULL,
    biz_type         varchar(32),
    title            varchar(255),
    prompt_template  text,
    status           varchar(16)  NOT NULL DEFAULT 'active',
    created_at       timestamptz  NOT NULL DEFAULT now(),
    updated_at       timestamptz  NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX ai_sessions_code_uniq  ON ai_sessions (code);
CREATE INDEX        ai_sessions_user_idx   ON ai_sessions (user_id);
CREATE INDEX        ai_sessions_ctx_idx    ON ai_sessions (class_id, node_id);
CREATE INDEX        ai_sessions_active_idx ON ai_sessions (created_at DESC) WHERE status = 'active';

-- ============== ai_messages AI 消息 ==============
CREATE TABLE ai_messages (
    id                 bigserial   PRIMARY KEY,
    session_id         bigint      NOT NULL REFERENCES ai_sessions(id) ON DELETE CASCADE,
    role               ai_message_role NOT NULL,
    content            text        NOT NULL,
    finish_reason      chat_finish_reason,
    prompt_tokens      int,
    completion_tokens  int,
    total_tokens       int,
    model_name         varchar(64),
    seq                int         NOT NULL,
    created_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ai_messages_session_seq_idx  ON ai_messages (session_id, seq);
CREATE INDEX ai_messages_session_time_idx ON ai_messages (session_id, created_at);

-- ============== ai_search_references AI 引用 ==============
CREATE TABLE ai_search_references (
    id          bigserial   PRIMARY KEY,
    message_id  bigint      NOT NULL REFERENCES ai_messages(id) ON DELETE CASCADE,
    ref_index   int         NOT NULL DEFAULT 0,
    title       varchar(255) NOT NULL,
    link        text        NOT NULL,
    snippet     text,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ai_search_references_msg_idx ON ai_search_references (message_id);

-- ============== moments 朋友圈动态 ==============
-- 取代旧 work_publishes；objs_json 存 MomentExtraObj 数组
CREATE TABLE moments (
    id              bigserial    PRIMARY KEY,
    class_id        bigint       NOT NULL REFERENCES classes(id) ON DELETE CASCADE,
    user_id         bigint       NOT NULL REFERENCES users(id),
    source          varchar(64),
    content         text         NOT NULL,
    objs_json       jsonb        NOT NULL DEFAULT '[]'::jsonb,
    like_count      int          NOT NULL DEFAULT 0,   -- 冗余计数，提升查询性能
    comment_count   int          NOT NULL DEFAULT 0,
    created_at      timestamptz  NOT NULL DEFAULT now(),
    updated_at      timestamptz  NOT NULL DEFAULT now(),
    deleted_at      timestamptz
);
CREATE INDEX moments_class_idx ON moments (class_id, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX moments_user_idx  ON moments (user_id, created_at DESC) WHERE deleted_at IS NULL;

-- ============== moment_images 朋友圈图片 ==============
CREATE TABLE moment_images (
    id           bigserial   PRIMARY KEY,
    moment_id    bigint      NOT NULL REFERENCES moments(id) ON DELETE CASCADE,
    resource_id  bigint      NOT NULL REFERENCES resources(id),
    sort_order   int         NOT NULL DEFAULT 0,
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (moment_id, resource_id)
);
CREATE INDEX moment_images_resource_idx ON moment_images (resource_id);

-- ============== moment_likes 朋友圈点赞 ==============
CREATE TABLE moment_likes (
    id          bigserial   PRIMARY KEY,
    moment_id   bigint      NOT NULL REFERENCES moments(id) ON DELETE CASCADE,
    user_id     bigint      NOT NULL REFERENCES users(id),
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (moment_id, user_id)
);
CREATE INDEX moment_likes_user_idx ON moment_likes (user_id);

-- ============== moment_comments 朋友圈评论 ==============
CREATE TABLE moment_comments (
    id          bigserial   PRIMARY KEY,
    moment_id   bigint      NOT NULL REFERENCES moments(id) ON DELETE CASCADE,
    user_id     bigint      NOT NULL REFERENCES users(id),
    content     text        NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz
);
CREATE INDEX moment_comments_moment_idx ON moment_comments (moment_id, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX moment_comments_user_idx   ON moment_comments (user_id);

-- ============== updated_at 触发器 ==============
CREATE OR REPLACE FUNCTION touch_updated_at() RETURNS trigger AS $$
BEGIN
    NEW.updated_at := now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- 为所有有 updated_at 的表挂触发器
DO $$
DECLARE t text;
BEGIN
    FOR t IN
        SELECT table_name FROM information_schema.columns
        WHERE column_name = 'updated_at' AND table_schema = 'public'
    LOOP
        EXECUTE format(
            'CREATE TRIGGER set_updated_at BEFORE UPDATE ON %I FOR EACH ROW EXECUTE FUNCTION touch_updated_at();',
            t
        );
    END LOOP;
END $$;
