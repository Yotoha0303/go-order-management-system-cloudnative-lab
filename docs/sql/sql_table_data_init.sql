-- seed_day01.sql
-- 适配当前项目：go-order-management-system
-- 当前表：products / product_inventories / stock_logs

SET NAMES utf8mb4;

SET FOREIGN_KEY_CHECKS = 0;

TRUNCATE TABLE stock_logs;

TRUNCATE TABLE product_inventories;

TRUNCATE TABLE products;

TRUNCATE TABLE order_items;

TRUNCATE TABLE orders;

SET FOREIGN_KEY_CHECKS = 1;

-- 1. 商品测试数据
-- status: 1=上架, 2=下架
-- price_fen: 以分为单位，例如 19900 = 199.00 元
INSERT INTO
    products (
        id,
        name,
        description,
        price_fen,
        status,
        created_at,
        updated_at
    )
VALUES (
        1,
        '机械键盘 K87',
        '入门级 87 键机械键盘，适合办公和编程',
        19900,
        1,
        NOW(),
        NOW()
    ),
    (
        2,
        '无线鼠标 M1',
        '轻量无线鼠标，适合日常办公',
        8900,
        1,
        NOW(),
        NOW()
    ),
    (
        3,
        'USB-C 扩展坞',
        '多接口扩展坞，支持 HDMI、USB、网口',
        15900,
        1,
        NOW(),
        NOW()
    ),
    (
        4,
        '27寸显示器',
        '27寸 2K 办公显示器',
        89900,
        1,
        NOW(),
        NOW()
    ),
    (
        5,
        '笔记本支架',
        '铝合金可调节笔记本支架',
        6900,
        1,
        NOW(),
        NOW()
    ),
    (
        6,
        '降噪耳机',
        '头戴式蓝牙降噪耳机',
        29900,
        1,
        NOW(),
        NOW()
    ),
    (
        7,
        '移动硬盘 1TB',
        '1TB USB 3.0 移动硬盘',
        39900,
        1,
        NOW(),
        NOW()
    ),
    (
        8,
        '手机充电器 65W',
        '65W GaN 快充充电器',
        12900,
        1,
        NOW(),
        NOW()
    ),
    (
        9,
        '双肩电脑包',
        '可放 15.6 寸笔记本电脑的通勤背包',
        13900,
        2,
        NOW(),
        NOW()
    ),
    (
        10,
        '桌面收纳架',
        '桌面文件和配件收纳架',
        4900,
        2,
        NOW(),
        NOW()
    );

-- 2. 库存测试数据
-- 当前表名来自 Inventory.TableName(): product_inventories
INSERT INTO
    product_inventories (
        id,
        product_id,
        stock_quantity,
        created_at,
        updated_at
    )
VALUES (1, 1, 120, NOW(), NOW()),
    (2, 2, 200, NOW(), NOW()),
    (3, 3, 80, NOW(), NOW()),
    (4, 4, 35, NOW(), NOW()),
    (5, 5, 150, NOW(), NOW()),
    (6, 6, 60, NOW(), NOW()),
    (7, 7, 45, NOW(), NOW()),
    (8, 8, 100, NOW(), NOW()),
    (9, 9, 30, NOW(), NOW()),
    (10, 10, 180, NOW(), NOW());

-- 3. 库存流水测试数据
-- biz_type:
-- 1=初始化库存
-- 2=手动入库
-- 3=下单扣减
-- 4=订单取消回滚
INSERT INTO
    stock_logs (
        id,
        product_id,
        change_quantity,
        before_quantity,
        after_quantity,
        biz_type,
        biz_id,
        remark,
        created_at
    )
VALUES
    -- 初始化库存
    (
        1,
        1,
        100,
        0,
        100,
        1,
        NULL,
        '初始化库存：机械键盘 K87',
        NOW()
    ),
    (
        2,
        2,
        180,
        0,
        180,
        1,
        NULL,
        '初始化库存：无线鼠标 M1',
        NOW()
    ),
    (
        3,
        3,
        60,
        0,
        60,
        1,
        NULL,
        '初始化库存：USB-C 扩展坞',
        NOW()
    ),
    (
        4,
        4,
        30,
        0,
        30,
        1,
        NULL,
        '初始化库存：27寸显示器',
        NOW()
    ),
    (
        5,
        5,
        120,
        0,
        120,
        1,
        NULL,
        '初始化库存：笔记本支架',
        NOW()
    ),
    (
        6,
        6,
        50,
        0,
        50,
        1,
        NULL,
        '初始化库存：降噪耳机',
        NOW()
    ),
    (
        7,
        7,
        40,
        0,
        40,
        1,
        NULL,
        '初始化库存：移动硬盘 1TB',
        NOW()
    ),
    (
        8,
        8,
        90,
        0,
        90,
        1,
        NULL,
        '初始化库存：手机充电器 65W',
        NOW()
    ),
    (
        9,
        9,
        30,
        0,
        30,
        1,
        NULL,
        '初始化库存：双肩电脑包',
        NOW()
    ),
    (
        10,
        10,
        160,
        0,
        160,
        1,
        NULL,
        '初始化库存：桌面收纳架',
        NOW()
    ),

-- 手动入库
(
    11,
    1,
    20,
    100,
    120,
    2,
    NULL,
    '手动入库：补充机械键盘库存',
    NOW()
),
(
    12,
    2,
    20,
    180,
    200,
    2,
    NULL,
    '手动入库：补充无线鼠标库存',
    NOW()
),
(
    13,
    3,
    20,
    60,
    80,
    2,
    NULL,
    '手动入库：补充扩展坞库存',
    NOW()
),
(
    14,
    4,
    5,
    30,
    35,
    2,
    NULL,
    '手动入库：补充显示器库存',
    NOW()
),
(
    15,
    5,
    30,
    120,
    150,
    2,
    NULL,
    '手动入库：补充笔记本支架库存',
    NOW()
),
(
    16,
    6,
    10,
    50,
    60,
    2,
    NULL,
    '手动入库：补充耳机库存',
    NOW()
),
(
    17,
    7,
    5,
    40,
    45,
    2,
    NULL,
    '手动入库：补充移动硬盘库存',
    NOW()
),
(
    18,
    8,
    10,
    90,
    100,
    2,
    NULL,
    '手动入库：补充充电器库存',
    NOW()
),
(
    19,
    10,
    20,
    160,
    180,
    2,
    NULL,
    '手动入库：补充桌面收纳架库存',
    NOW()
);
