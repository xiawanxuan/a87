-- ============================================
-- 古建筑木构件超声波探伤标注系统 - 迁移脚本 v2
-- 功能：新增像素-实际尺寸换算比例字段
-- 执行时间：约 1 秒
-- ============================================

-- 1. 为 scan_images 表添加 pixel_scale_mm 字段
ALTER TABLE scan_images
    ADD COLUMN IF NOT EXISTS pixel_scale_mm DECIMAL(10,6) NOT NULL DEFAULT 0.1;

-- 2. 添加注释
COMMENT ON COLUMN scan_images.pixel_scale_mm IS '像素换算比例：毫米/像素 (mm/px)';

-- 3. 更新已有的数据，设置合理的默认值
--    4K 图像（4096像素）对应约 60cm 尺寸的木构件
--    600mm / 4096px ≈ 0.146 mm/px
UPDATE scan_images
    SET pixel_scale_mm = 0.15
    WHERE width >= 3000 AND pixel_scale_mm = 0.1;

-- 4. 重新计算已有标注的实际面积（如果 area_cm2 为 0 或空）
--    注意：这是一个估算，实际应该调用 service 层重新计算
UPDATE polygon_annotations pa
    SET area_cm2 = ROUND(
        (pa.area_px * si.pixel_scale_mm * si.pixel_scale_mm) / 100.0,
        4
    )
    FROM scan_images si
    WHERE pa.scan_image_id = si.id
      AND (pa.area_cm2 IS NULL OR pa.area_cm2 = 0)
      AND pa.area_px > 0
      AND si.pixel_scale_mm > 0;

-- ============================================
-- 迁移完成
-- ============================================
