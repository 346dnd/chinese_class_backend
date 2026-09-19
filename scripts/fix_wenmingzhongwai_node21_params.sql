-- ============================================================
-- 修复：文明中外（node 21，node_contents id=28）params_json 缺少 correct-answer 单元格
-- 现象：第一题填"热闹"也判错（correctAnswer 为空导致 gradeRow 恒 false）
-- 修复：将 params_json 更新为与 scripts/seed.sql 一致的、含 correct-answer 的版本
-- 用法：psql -U postgres -d zhonghuawenhua -f scripts/fix_wenmingzhongwai_node21_params.sql
-- ============================================================

UPDATE node_contents
SET params_json = '{
  "title": "宣传有法：学习《一幅名扬中外的画》的表达方法",
  "introVideo": { "autoPlay": false, "url": "/static/videos/wenmingzhongwai-intro.mp4" },
  "introBubbleText": "请仔细阅读第3自然段，沿着作者的思路在空格中填入文字。",
  "description": "请仔细阅读《一幅名扬中外的画》第3自然段，沿着作者描写的思路，在对应的空格中填入文字。",
  "matrix": {
    "columns": [
      { "key": "col1", "title": "题目" },
      { "key": "col2", "title": "答案" }
    ],
    "rows": [
      {
        "rowId": "r1",
        "cells": [
          { "columnKey": "col1", "type": "text", "content": "怎么写" },
          { "columnKey": "col2", "type": "text", "content": "《一幅名扬中外的画》第 3 自然段" }
        ]
      },
      {
        "rowId": "r2",
        "cells": [
          { "columnKey": "col1", "type": "text", "content": "①先确定一个意思。" },
          { "columnKey": "col2", "type": "input", "content": "" },
          { "columnKey": "col2", "type": "correct-answer", "content": "热闹" }
        ]
      },
      {
        "rowId": "r3",
        "cells": [
          { "columnKey": "col1", "type": "text", "content": "②根据这个意思写一句中心句。" },
          { "columnKey": "col2", "type": "input", "content": "" },
          { "columnKey": "col2", "type": "correct-answer", "content": "画上的街市可热闹了" }
        ]
      },
      {
        "rowId": "r4",
        "cells": [
          { "columnKey": "col1", "type": "text", "content": "③围绕中心句，后面每一句话写的内容都跟这个意思有关。可以用上修辞手法，可以用事例或细节来写具体。" },
          { "columnKey": "col2", "type": "input", "content": "" },
          { "columnKey": "col2", "type": "correct-answer", "content": "街上有挂着各种招牌的店铺。走在街上的，是来来往往、形态各异的人：有的骑着马，有的挑着担，有的赶着毛驴，有的推着独轮车，有的悠闲地在街上溜达。" }
        ]
      }
    ]
  }
}',
version = 1
WHERE id = 28 AND node_id = 21;
