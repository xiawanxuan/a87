-- ============================================
-- 古建筑木构件超声波探伤标注系统 - 演示数据
-- ============================================

-- 构件分类
INSERT INTO component_categories (name, description) VALUES
    ('梁',  '水平受力构件'),
    ('柱',  '垂直受力构件'),
    ('斗拱', '过渡受力构件'),
    ('檩条', '屋面承重构件'),
    ('椽',  '椽子，屋面基层构件'),
    ('枋',  '辅助联系构件'),
    ('垫板', '填充构件')
ON CONFLICT (name) DO NOTHING;

-- 病害类型已在 001 初始化脚本中插入
-- 虫蛀(wormhole)、干缩裂纹(crack)、腐朽(decay)、空鼓(hollowing)

-- 构建木构件目录树
-- 一级：建筑
INSERT INTO wood_components (parent_id, category_id, name, code, building_name, location, description, material) VALUES
    (NULL, NULL, '故宫太和殿', 'THD-001', '故宫太和殿', '北京紫禁城中路', '太和殿主体木构系统', '金丝楠木'),
    (NULL, NULL, '故宫保和殿', 'BHD-001', '故宫保和殿', '北京紫禁城中路', '保和殿主体木构系统', '松木'),
    (NULL, NULL, '应县木塔', 'YXMT-001', '应县佛宫寺释迦塔', '山西应县', '辽代木塔，世界最高木构建筑', '松木')
ON CONFLICT (code) DO NOTHING;

-- 二级：太和殿主要结构
WITH bld AS (SELECT id FROM wood_components WHERE code = 'THD-001'),
cats AS (SELECT id, code, name FROM component_categories WHERE code IN ('liang', 'zhu', 'dougong', 'lintiao', 'chuan'))
INSERT INTO wood_components (parent_id, category_id, name, code, building_name, location, description, material)
SELECT bld.id, cats.id,
       '太和殿' || cats.name || '-' || CASE cats.code
           WHEN 'liang' THEN '前檐明间'
           WHEN 'zhu' THEN '前檐东一'
           WHEN 'dougong' THEN '平身科东一'
           WHEN 'lintiao' THEN '正脊东段'
           WHEN 'chuan' THEN '上檐东'
           ELSE '主'
       END,
       'THD-' || cats.code || '-01',
       '故宫太和殿',
       '前檐明间',
       cats.name || '示例构件 - 供演示标注使用',
       CASE cats.code
           WHEN 'liang' THEN '金丝楠木'
           WHEN 'zhu' THEN '金丝楠木'
           ELSE '松木'
       END
FROM bld, cats
ON CONFLICT (code) DO NOTHING;

-- 三级：太和殿梁子构件
WITH parent AS (SELECT id FROM wood_components WHERE code = 'THD-liang-01'),
positions AS (
    SELECT '东一跨' as pos, 'THD-liang-01-01' as code UNION ALL
    SELECT '东二跨' as pos, 'THD-liang-01-02' as code UNION ALL
    SELECT '西一跨' as pos, 'THD-liang-01-03' as code UNION ALL
    SELECT '西二跨' as pos, 'THD-liang-01-04' as code
)
INSERT INTO wood_components (parent_id, category_id, name, code, building_name, location, description, material)
SELECT parent.id, c.id, '太和殿梁-' || positions.pos, positions.code,
       '故宫太和殿', '前檐明间', '梁架结构-' || positions.pos, '金丝楠木'
FROM parent, positions
CROSS JOIN component_categories c WHERE c.code = 'liang'
ON CONFLICT (code) DO NOTHING;

-- 插入示例探伤图元数据（实际图片需通过系统上传）
-- 为演示，我们创建一些模拟的4K探伤图记录
-- pixel_scale_mm: 0.15 mm/px 表示每像素对应 0.15 毫米
-- 4096像素 × 0.15mm = 614.4mm ≈ 61.4cm（约为一根大梁的横截面尺寸）
WITH comp AS (SELECT id, code FROM wood_components WHERE code LIKE 'THD-liang-01-0%' LIMIT 4),
imgs AS (
    SELECT comp.id, comp.code,
           '超声探伤图_' || comp.code || '_正面.png' as fname,
           4096 as w, 4096 as h,
           8 as bits,
           0.15 as pscale,
           '2024-01-15'::date as sdate,
           '张工' as op,
           'USM-36' as equip,
           '正面扫查，无盲区' as remark
    FROM comp
)
INSERT INTO scan_images (
    component_id, file_name, file_path, file_size,
    mime_type, width, height, bits_depth, pixel_scale_mm,
    scan_date, operator, equipment, remark, uploaded_by
) SELECT
    imgs.id, imgs.fname,
    './uploads/demo/' || imgs.code || '/scan_front.png',
    16 * 1024 * 1024,
    'image/png',
    imgs.w, imgs.h, imgs.bits, imgs.pscale,
    imgs.sdate, imgs.op, imgs.equip, imgs.remark, 'admin'
FROM imgs
ON CONFLICT DO NOTHING;

-- 插入第二批探伤图（侧面扫查）
WITH comp AS (SELECT id, code FROM wood_components WHERE code LIKE 'THD-liang-01-0%' LIMIT 4),
imgs AS (
    SELECT comp.id, comp.code,
           '超声探伤图_' || comp.code || '_侧面.png' as fname,
           4096 as w, 2048 as h,
           8 as bits,
           0.20 as pscale,
           '2024-01-15'::date as sdate,
           '张工' as op,
           'USM-36' as equip,
           '侧面扫查，补充检测' as remark
    FROM comp
)
INSERT INTO scan_images (
    component_id, file_name, file_path, file_size,
    mime_type, width, height, bits_depth, pixel_scale_mm,
    scan_date, operator, equipment, remark, uploaded_by
) SELECT
    imgs.id, imgs.fname,
    './uploads/demo/' || imgs.code || '/scan_side.png',
    8 * 1024 * 1024,
    'image/png',
    imgs.w, imgs.h, imgs.bits, imgs.pscale,
    imgs.sdate, imgs.op, imgs.equip, imgs.remark, 'admin'
FROM imgs
ON CONFLICT DO NOTHING;

-- 创建演示标注数据（如果有对应的图像ID）
-- 注意：实际标注需要通过前端创建，这里仅创建架构演示

-- ============================================
-- 初始化完成
-- ============================================
