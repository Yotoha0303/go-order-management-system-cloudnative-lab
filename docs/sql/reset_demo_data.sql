-- Active: 1778141354927@@127.0.0.1@3306@go_order_inventory_demo
-- 删除表数据
DELETE FROM stock_logs;

DELETE FROM order_items;

DELETE FROM orders;

DELETE FROM product_inventories;

DELETE FROM products;

-- 删除表 - 用于删除 id 的自增属性
DROP TABLE stock_logs;

DROP TABLE order_items;

DROP TABLE orders;

DROP TABLE product_inventories;

DROP TABLE products;