#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
古建筑木构件超声波探伤灰度图谱 - 演示图像生成器
生成模拟的4K灰度超声探伤图，包含模拟的病害区域
"""

import os
import sys
import argparse
import numpy as np
from PIL import Image, ImageDraw, ImageFilter


def generate_ultrasound_image(width=4096, height=4096, seed=42):
    """生成模拟的超声波探伤灰度图"""
    np.random.seed(seed)

    # 1. 创建基础灰度背景 - 模拟木材纹理
    img = np.zeros((height, width), dtype=np.float32)
    base_gray = 120
    img += base_gray

    # 2. 添加木纹纹理 - 纵向条纹
    for i in range(width):
        intensity = np.random.normal(0, 8)
        img[:, i] += intensity
        if i % 30 == 0:
            img[:, max(0, i - 2):min(width, i + 2)] += np.random.normal(15, 5)

    # 3. 添加水平纹理
    for j in range(height):
        if j % 50 == 0:
            intensity = np.random.normal(5, 3)
            img[max(0, j - 3):min(height, j + 3), :] += intensity

    # 4. 添加整体噪声
    noise = np.random.normal(0, 6, (height, width))
    img += noise

    # 5. 添加模拟的病害区域
    defects = []

    # 虫蛀区域 (wormhole) - 多个小圆孔
    num_wormholes = np.random.randint(8, 20)
    for _ in range(num_wormholes):
        cx = np.random.randint(200, width - 200)
        cy = np.random.randint(200, height - 200)
        radius = np.random.randint(15, 50)
        defects.append(('wormhole', cx, cy, radius))

        y, x = np.ogrid[:height, :width]
        dist = np.sqrt((x - cx) ** 2 + (y - cy) ** 2)
        mask = dist < radius
        img[mask] -= np.random.randint(40, 80)

    # 干缩裂纹 (crack) - 细长暗色线条
    num_cracks = np.random.randint(3, 8)
    for _ in range(num_cracks):
        cx = np.random.randint(300, width - 300)
        cy = np.random.randint(300, height - 300)
        length = np.random.randint(200, 800)
        angle = np.random.uniform(-np.pi / 4, np.pi / 4)
        width_crack = np.random.randint(3, 10)
        defects.append(('crack', cx, cy, length, angle, width_crack))

        for t in np.linspace(-length / 2, length / 2, int(length)):
            x = int(cx + t * np.cos(angle))
            y = int(cy + t * np.sin(angle) + np.random.normal(0, 3))
            if 0 <= x < width and 0 <= y < height:
                w = int(width_crack + np.random.normal(0, 2))
                img[max(0, y - w):min(height, y + w),
                    max(0, x - w):min(width, x + w)] -= np.random.randint(30, 60)

    # 腐朽区域 (decay) - 大面积不规则暗区
    num_decays = np.random.randint(2, 5)
    for _ in range(num_decays):
        cx = np.random.randint(400, width - 400)
        cy = np.random.randint(400, height - 400)
        radius = np.random.randint(80, 200)
        defects.append(('decay', cx, cy, radius))

        y, x = np.ogrid[:height, :width]
        dist = np.sqrt((x - cx) ** 2 + (y - cy) ** 2)
        mask = dist < radius
        noise = np.random.uniform(0.5, 1.5, mask.shape)
        mask = mask & (noise * dist < radius)
        img[mask] -= np.random.randint(20, 50)

    # 空鼓区域 (hollowing) - 较亮的区域，内部有阴影
    num_hollowing = np.random.randint(1, 4)
    for _ in range(num_hollowing):
        cx = np.random.randint(500, width - 500)
        cy = np.random.randint(500, height - 500)
        radius = np.random.randint(60, 150)
        defects.append(('hollowing', cx, cy, radius))

        y, x = np.ogrid[:height, :width]
        dist = np.sqrt((x - cx) ** 2 + (y - cy) ** 2)
        mask = dist < radius
        img[mask] += np.random.randint(15, 40)
        inner_mask = (dist > radius * 0.7) & (dist < radius)
        img[inner_mask] -= np.random.randint(10, 25)

    # 6. 裁剪到有效范围 0-255
    img = np.clip(img, 0, 255).astype(np.uint8)

    # 7. 轻微模糊，模拟超声探头特性
    img_pil = Image.fromarray(img)
    img_pil = img_pil.filter(ImageFilter.GaussianBlur(radius=0.8))

    return img_pil, defects


def save_demo_images(output_dir='./demo_images', num_images=4):
    """生成多张演示图像"""
    os.makedirs(output_dir, exist_ok=True)

    defect_types = {
        'wormhole': '虫蛀',
        'crack': '干缩裂纹',
        'decay': '腐朽',
        'hollowing': '空鼓',
    }

    for i in range(num_images):
        print(f"生成演示图像 {i + 1}/{num_images}...")
        width = 4096 if i % 2 == 0 else 3840
        height = 4096 if i % 2 == 0 else 2160

        img, defects = generate_ultrasound_image(width, height, seed=100 + i)

        filename = f"demo_scan_{i + 1:02d}_{width}x{height}.png"
        filepath = os.path.join(output_dir, filename)
        img.save(filepath, 'PNG', optimize=True)

        print(f"  保存到: {filepath}")
        print(f"  尺寸: {width}x{height}")
        print(f"  模拟病害:")
        for d in defects:
            dtype = d[0]
            info = f"位置({d[1]},{d[2]})"
            if dtype == 'crack':
                info += f" 长度{d[3]}px 角度{d[4]:.2f}rad"
            else:
                info += f" 半径{d[3]}px"
            print(f"    - {defect_types.get(dtype, dtype)}: {info}")
        print()

    print(f"完成！共生成 {num_images} 张演示图像到 {output_dir}/")


def generate_uploads_structure(base_dir='../../backend/uploads', num_components=4):
    """生成与 seed 数据对应的上传目录结构"""
    codes = ['THD-liang-01-01', 'THD-liang-01-02', 'THD-liang-01-03', 'THD-liang-01-04']
    for i in range(min(num_components, len(codes))):
        code = codes[i]
        comp_dir = os.path.join(base_dir, code)
        os.makedirs(comp_dir, exist_ok=True)

        print(f"生成 {code} 探伤图...")
        width, height = 4096, 4096
        img_front, _ = generate_ultrasound_image(width, height, seed=200 + i * 2)
        img_side, _ = generate_ultrasound_image(4096, 2048, seed=200 + i * 2 + 1)

        front_path = os.path.join(comp_dir, 'scan_front.png')
        side_path = os.path.join(comp_dir, 'scan_side.png')
        img_front.save(front_path, 'PNG', optimize=True)
        img_side.save(side_path, 'PNG', optimize=True)
        print(f"  - {front_path} ({width}x{height})")
        print(f"  - {side_path} (4096x2048)")

    print("上传目录结构创建完成！")


def main():
    parser = argparse.ArgumentParser(
        description='古建筑木构件超声波探伤灰度图谱 - 演示图像生成工具'
    )
    parser.add_argument(
        '-o', '--output', default='./demo_images',
        help='演示图像输出目录 (默认: ./demo_images)'
    )
    parser.add_argument(
        '-n', '--num', type=int, default=4,
        help='生成演示图像数量 (默认: 4)'
    )
    parser.add_argument(
        '--uploads', action='store_true',
        help='同时创建 backend/uploads 目录结构并填充对应图像'
    )
    parser.add_argument(
        '--uploads-dir', default='../../backend/uploads',
        help='后端上传目录路径 (默认: ../../backend/uploads)'
    )

    args = parser.parse_args()

    print("=" * 60)
    print("  古建筑木构件超声波探伤灰度图谱 - 演示图像生成器")
    print("=" * 60)
    print()

    save_demo_images(args.output, args.num)

    if args.uploads:
        print()
        print("=" * 60)
        print("  创建后端上传目录结构...")
        print("=" * 60)
        generate_uploads_structure(args.uploads_dir)

    return 0


if __name__ == '__main__':
    sys.exit(main())
