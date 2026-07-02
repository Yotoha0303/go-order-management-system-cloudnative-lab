SELECT * FROM products ORDER BY id;

SELECT * FROM product_inventories ORDER BY product_id;

SELECT product_id, SUM(change_quantity) AS log_total_change
FROM stock_logs
GROUP BY
    product_id
ORDER BY product_id;

SELECT
    p.id AS product_id,
    p.name,
    pi.stock_quantity,
    COALESCE(SUM(sl.change_quantity), 0) AS log_quantity_sum,
    CASE
        WHEN pi.stock_quantity = COALESCE(SUM(sl.change_quantity), 0) THEN 'OK'
        ELSE 'MISMATCH'
    END AS check_result
FROM
    products p
    LEFT JOIN product_inventories pi ON pi.product_id = p.id
    LEFT JOIN stock_logs sl ON sl.product_id = p.id
GROUP BY
    p.id,
    p.name,
    pi.stock_quantity
ORDER BY p.id;