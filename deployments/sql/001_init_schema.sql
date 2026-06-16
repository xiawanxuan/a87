-- ============================================
-- 古建筑木构件超声波探伤标注系统 - 数据库初始化脚本
-- ============================================

-- 构件分类表
CREATE TABLE IF NOT EXISTS component_categories (
    id              BIGSERIAL PRIMARY KEY,
    name            VARCHAR(64)  NOT NULL UNIQUE,
    description     TEXT,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- 木构件表（目录树节点）
CREATE TABLE IF NOT EXISTS wood_components (
    id              BIGSERIAL PRIMARY KEY,
    parent_id       BIGINT REFERENCES wood_components(id) ON DELETE CASCADE,
    category_id     BIGINT REFERENCES component_categories(id),
    name            VARCHAR(128) NOT NULL,
    code            VARCHAR(64)  UNIQUE,
    description     TEXT,
    building_name   VARCHAR(128),
    location        VARCHAR(256),
    material        VARCHAR(64),
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_wood_components_parent ON wood_components(parent_id);
CREATE INDEX IF NOT EXISTS idx_wood_components_tree_path ON wood_components USING btree (parent_id, id);

-- 病害类型表
CREATE TABLE IF NOT EXISTS disease_types (
    id              BIGSERIAL PRIMARY KEY,
    code            VARCHAR(32)  NOT NULL UNIQUE,
    name            VARCHAR(64)  NOT NULL,
    color_hex       VARCHAR(16)  NOT NULL DEFAULT '#FF6B6B',
    description     TEXT,
    severity_order  INT          NOT NULL DEFAULT 0,
    enabled         BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

INSERT INTO disease_types (code, name, color_hex, severity_order, description) VALUES
    ('wormhole',   '虫蛀',   '#FF6B6B', 1, '木材被蛀虫侵蚀形成的孔洞或蛀道'),
    ('crack',      '干缩裂纹', '#4ECDC4', 2, '木材因干燥收缩产生的开裂'),
    ('decay',      '腐朽',   '#95E1D3', 3, '木材受真菌侵蚀导致材质劣化'),
    ('hollowing',  '空鼓',   '#FFD93D', 4, '木材内部脱层形成的空洞区域')
ON CONFLICT (code) DO NOTHING;

-- 探伤图元数据表
CREATE TABLE IF NOT EXISTS scan_images (
    id              BIGSERIAL PRIMARY KEY,
    component_id    BIGINT       NOT NULL REFERENCES wood_components(id) ON DELETE CASCADE,
    file_name       VARCHAR(256) NOT NULL,
    file_path       VARCHAR(512) NOT NULL,
    file_size       BIGINT       NOT NULL DEFAULT 0,
    mime_type       VARCHAR(64),
    width           INT          NOT NULL DEFAULT 0,
    height          INT          NOT NULL DEFAULT 0,
    bits_depth      INT          NOT NULL DEFAULT 8,
    pixel_scale_mm  DECIMAL(10,6) NOT NULL DEFAULT 0.1,
    scan_date       DATE,
    operator        VARCHAR(64),
    equipment       VARCHAR(128),
    remark          TEXT,
    uploaded_by     VARCHAR(64),
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_scan_images_component ON scan_images(component_id);
CREATE INDEX IF NOT EXISTS idx_scan_images_created ON scan_images(created_at DESC);

-- 标注图层（用于多人协同分层管理）
CREATE TABLE IF NOT EXISTS annotation_layers (
    id              BIGSERIAL PRIMARY KEY,
    scan_image_id   BIGINT       NOT NULL REFERENCES scan_images(id) ON DELETE CASCADE,
    layer_name      VARCHAR(64)  NOT NULL,
    disease_type_id BIGINT REFERENCES disease_types(id),
    opacity         FLOAT        NOT NULL DEFAULT 0.6,
    visible         BOOLEAN      NOT NULL DEFAULT TRUE,
    locked          BOOLEAN      NOT NULL DEFAULT FALSE,
    sort_order      INT          NOT NULL DEFAULT 0,
    created_by      VARCHAR(64),
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ann_layers_image ON annotation_layers(scan_image_id);

-- 病害多边形标注表
CREATE TABLE IF NOT EXISTS polygon_annotations (
    id              BIGSERIAL PRIMARY KEY,
    scan_image_id   BIGINT       NOT NULL REFERENCES scan_images(id) ON DELETE CASCADE,
    layer_id        BIGINT REFERENCES annotation_layers(id) ON DELETE SET NULL,
    disease_type_id BIGINT       NOT NULL REFERENCES disease_types(id),
    label           VARCHAR(256),
    points          JSONB        NOT NULL,
    bounding_box    JSONB,
    area_px         FLOAT,
    area_cm2        FLOAT,
    severity        INT          NOT NULL DEFAULT 1 CHECK (severity BETWEEN 1 AND 5),
    note            TEXT,
    operator        VARCHAR(64),
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_poly_ann_image ON polygon_annotations(scan_image_id);
CREATE INDEX IF NOT EXISTS idx_poly_ann_layer ON polygon_annotations(layer_id);
CREATE INDEX IF NOT EXISTS idx_poly_ann_disease ON polygon_annotations(disease_type_id);
CREATE INDEX IF NOT EXISTS idx_poly_ann_points ON polygon_annotations USING GIN (points);

-- 标注版本快照表
CREATE TABLE IF NOT EXISTS annotation_snapshots (
    id              BIGSERIAL PRIMARY KEY,
    scan_image_id   BIGINT       NOT NULL REFERENCES scan_images(id) ON DELETE CASCADE,
    version_number  INT          NOT NULL,
    snapshot_data   JSONB        NOT NULL,
    diff_summary    TEXT,
    operator        VARCHAR(64),
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_snapshot_image_version ON annotation_snapshots(scan_image_id, version_number);
CREATE INDEX IF NOT EXISTS idx_snapshot_created ON annotation_snapshots(scan_image_id, created_at DESC);

-- 在线协同编辑会话状态（可选持久化）
CREATE TABLE IF NOT EXISTS collaboration_sessions (
    id              BIGSERIAL PRIMARY KEY,
    scan_image_id   BIGINT       NOT NULL REFERENCES scan_images(id) ON DELETE CASCADE,
    user_id         VARCHAR(64)  NOT NULL,
    user_name       VARCHAR(64),
    cursor_pos      JSONB,
    active_tool     VARCHAR(32),
    last_heartbeat  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    joined_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (scan_image_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_collab_session_image ON collaboration_sessions(scan_image_id);
