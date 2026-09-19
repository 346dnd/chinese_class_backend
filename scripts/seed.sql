-- ============================================================
-- 中华文化教学后端 测试数据种子脚本
-- 对应 默认模块.openapi.json + migrations/000001_init_schema.up.sql
-- 约定：
--   * 所有业务主键 id 显式指定，全局递增（1,2,3,...）
--   * class_id 恒为 127
--   * 密码统一为 123456（bcrypt 占位，实际由代码插入）
-- 用法：psql -U postgres -d zhonghuawenhua -f scripts/seed.sql
-- ============================================================

BEGIN;

-- ============== users 用户（id 1-7） ==============
-- 密码占位：123456 的 bcrypt 哈希（$2a$10$... 示例），供登录测试替换
INSERT INTO users (id, code, username, password_hash, role, real_name, avatar_url, gender, city, grade_name, class_name, status) VALUES
(1, 'u-admin-001', 'admin',    '$2a$10$e0MYzXyjpJS7Pd0RVvHwHe1HlCkZtQdZQzZQzZQzZQzZQzZQzZQe', 'admin',   '管理员',   NULL, NULL, '深圳', '三年级', '三年1班', 1),
(2, 'u-teacher-001', 'teacher1', '$2a$10$e0MYzXyjpJS7Pd0RVvHwHe1HlCkZtQdZQzZQzZQzZQzZQzZQzZQe', 'teacher', '王老师',   NULL, '女', '深圳', '三年级', '三年1班', 1),
(3, 'u-stu-001',   'stu001',  '$2a$10$e0MYzXyjpJS7Pd0RVvHwHe1HlCkZtQdZQzZQzZQzZQzZQzZQzZQe', 'student', '张小满',   NULL, '男', '深圳', '三年级', '三年1班', 1),
(4, 'u-stu-002',   'stu002',  '$2a$10$e0MYzXyjpJS7Pd0RVvHwHe1HlCkZtQdZQzZQzZQzZQzZQzZQzZQe', 'student', '李思成',   NULL, '男', '深圳', '三年级', '三年1班', 1),
(5, 'u-stu-003',   'stu003',  '$2a$10$e0MYzXyjpJS7Pd0RVvHwHe1HlCkZtQdZQzZQzZQzZQzZQzZQzZQe', 'student', '王语嫣',   NULL, '女', '广州', '三年级', '三年1班', 1),
(6, 'u-stu-004',   'stu004',  '$2a$10$e0MYzXyjpJS7Pd0RVvHwHe1HlCkZtQdZQzZQzZQzZQzZQzZQzZQe', 'student', '赵子轩',   NULL, '男', '深圳', '三年级', '三年2班', 1),
(7, 'u-stu-005',   'stu005',  '$2a$10$e0MYzXyjpJS7Pd0RVvHwHe1HlCkZtQdZQzZQzZQzZQzZQzZQzZQe', 'student', '刘思颖',   NULL, '女', '北京', '三年级', '三年2班', 1);

-- ============== classes 班级（id 恒为 127） ==============
INSERT INTO classes (id, code, name, description, teacher_id, scene, start_at, end_at, status) VALUES
(127, 'cls-127-001', '三年级1班·中华文化探究', '三年级语文中华文化探究课堂', 2, 'inclass', now() - interval '30 days', now() + interval '30 days', 1);

-- ============== class_members 班级成员（id 8-14） ==============
INSERT INTO class_members (id, class_id, user_id, role) VALUES
(8,  127, 1, 'admin'),
(9,  127, 2, 'teacher'),
(10, 127, 3, 'student'),
(11, 127, 4, 'student'),
(12, 127, 5, 'student'),
(13, 127, 6, 'student'),
(14, 127, 7, 'student');

-- ============== class_nodes 课程节点（id 15-24，树形） ==============
-- 结构：根部 folder(15) → 活动文件夹(16) → 9 个叶子节点
INSERT INTO class_nodes (id, code, class_id, parent_id, node_type, title, description, sort_order, has_children, scene) VALUES
(15, 'n-root-001',        127, NULL, 'folder',              '中华文化探究课程',         '三年级语文中华文化探究', 0, true,  'inclass'),
(16, 'n-folder-activity', 127, 15,   'folder',              '课堂活动',                '包含全部学习活动节点',   1, true,  'inclass'),
-- 寻找文化：写写感想 / 初步感悟（initial-insight 为填空题页）
(17, 'n-write-thoughts',  127, 16,   'write-thoughts',      '寻找文化：写写感想',      '围绕一个意思写清楚想法', 1, false, 'inclass'),
(18, 'n-initial-insight', 127, 16,   'initial-insight',     '寻找文化：初步感悟',      '梳理表达方法，填空练习', 2, false, 'inclass'),
-- 重温文化：文化风格（含 quick-select / drag-sort 两种交互点的 breakpoints）
(19, 'n-cultural-style',  127, 16,   'cultural-style',      '重温文化：探秘纸的逆袭',  '互动视频 + 交互点题目',  3, false, 'inclass'),
-- 宣传有法：赵州桥 / 一幅名扬中外的画
(20, 'n-zhaozhouqiao',    127, 16,   'zhaozhouqiao',        '宣传有法：学习《赵州桥》', '朗读 + 文本回答',        4, false, 'inclass'),
(21, 'n-wenmingzhongwai', 127, 16,   'wenmingzhongwai',     '宣传有法：一幅名扬中外的画', '矩阵填空',               5, false, 'inclass'),
-- 宣传文化：讲解优秀文化（heritage-cultural 含 introBubbles）
(22, 'n-heritage',        127, 16,   'heritage-cultural',   '宣传文化：讲解优秀文化',  '跟读讲解 + AI 批改',     6, false, 'afterclass'),
-- 创作工坊：多种任务（generation/poemscripts/share/express）
(23, 'n-creation-ws',     127, 16,   'creation-workshop',   '创作工坊',               '手抄报/海报/古诗/剧本等创作', 7, false, 'inclass');

-- ============== node_contents 节点配置 params_json（id 24-30） ==============
-- 对应各 *Params schema：title / introVideo / introBubbleText / description 等

-- 24: 写写感想 WriteThoughtParams
INSERT INTO node_contents (id, node_id, version, params_json) VALUES
(24, 17, 1, '{
  "title": "寻找文化：写写感想",
  "introVideo": { "autoPlay": true, "url": "/static/videos/write-thoughts-intro.mp4" },
  "introBubbleText": "亲爱的某某同学，我们来梳理\u201c围绕一个意思把一段话写清楚\u201d的表达方法吧。你可以点击课文名称打开课文哦。",
  "subtitle": "写写感想",
  "description": [
    { "text": "这三篇课文都写到了中华优秀传统文化的内容，在《纸的发明》里，是哪些方面让你自豪？《赵州桥》《一幅名扬中外的画》又分别是哪些方面让你自豪？请把特别让你自豪的这些方面写下来，记得都要能够联系相应的课文内容和生活实际写想法。" }
  ],
  "lesson": "写写感想",
  "questions": [
    {
      "id": "q1",
      "title": "在《纸的发明》中，是哪些让你感到自豪？",
      "placeholder": "围绕一个意思写清楚，可以联系课文和生活实际……",
      "referenceAnswer": "示例：我为祖先的聪明才智感到自豪。他们发明了造纸术，让文字能传遍四方，这些成就让我感受到中华文化的伟大，也激励我努力学习。",
      "maxLength": 200,
      "maxErrors": 3
    },
    {
      "id": "q2",
      "title": "在《赵州桥》中，是哪些让你感到自豪？",
      "placeholder": "围绕一个意思写清楚，可以联系课文和生活实际……",
      "referenceAnswer": "示例：我为祖先的聪明才智感到自豪。他们修建了坚固的赵州桥，历经一千多年仍屹立不倒，这些成就让我感受到中华文化的伟大，也激励我努力学习。",
      "maxLength": 200,
      "maxErrors": 3
    },
    {
      "id": "q3",
      "title": "在《一幅闻名中外的画》中，是哪些让你感到自豪？",
      "placeholder": "围绕一个意思写清楚，可以联系课文和生活实际……",
      "referenceAnswer": "示例：我为祖先的聪明才智感到自豪。他们创作了《清明上河图》，生动记录了北宋都城汴京的繁华景象，这些成就让我感受到中华文化的伟大，也激励我努力学习。",
      "maxLength": 200,
      "maxErrors": 3
    }
  ]
}');

-- 25: 初步感悟 InitialImpressionsParams
INSERT INTO node_contents (id, node_id, version, params_json) VALUES
(25, 18, 1, '{
  "title": "寻找文化：初步感悟",
  "introVideo": { "autoPlay": true, "url": "/static/videos/initial-insight-intro.mp4" },
  "introBubbleText": "亲爱的某某同学，我们来梳理\u201c围绕一个意思把一段话写清楚\u201d的表达方法吧。你可以点击课文名称打开课文哦。",
  "subtitle": "初步感悟",
  "description": [{ "text": "亲爱的某某同学，来体会\u201c围绕一个意思把一段话写清楚\u201d的表达方法吧。" }],
  "questions": [
    {
      "id": "q1",
      "title": "一、在《赵州桥》的课文中",
      "content": [
        { "type": "text", "text": "作者详细介绍了桥面" },
        { "type": "blank", "blank": { "id": "b1", "placeholder": "点击输入", "referenceAnswer": "石栏", "maxErrors": 2 } },
        { "type": "text", "text": "上精美的" },
        { "type": "blank", "blank": { "id": "b2", "placeholder": "点击输入", "referenceAnswer": "图案", "maxErrors": 2 } },
        { "type": "text", "text": "，把各种" },
        { "type": "blank", "blank": { "id": "b3", "placeholder": "点击输入", "referenceAnswer": "姿态", "maxErrors": 2 } },
        { "type": "text", "text": "的" },
        { "type": "blank", "blank": { "id": "b4", "placeholder": "点击输入", "referenceAnswer": "龙", "maxErrors": 2 } },
        { "type": "text", "text": "写得活灵活现。" }
      ]
    },
    {
      "id": "q2",
      "title": "二、在《一幅名扬中外的画》的课文中",
      "content": [
        { "type": "text", "text": "作者先写" },
        { "type": "blank", "blank": { "id": "b1", "placeholder": "点击输入", "referenceAnswer": "店铺", "maxErrors": 2 } },
        { "type": "text", "text": "，再用上" },
        { "type": "blank", "blank": { "id": "b2", "placeholder": "点击输入", "referenceAnswer": "排比", "maxErrors": 2 } },
        { "type": "text", "text": "的修辞手法写来来往往、" },
        { "type": "blank", "blank": { "id": "b3", "placeholder": "点击输入", "referenceAnswer": "形态各异", "maxErrors": 2 } },
        { "type": "text", "text": "的人。" }
      ]
    }
  ]
}');

-- 26: 文化风格 CulturalStyleParams（含两处 breakpoints 交互点参数）
INSERT INTO node_contents (id, node_id, version, params_json) VALUES
(26, 19, 1, '{
  "title": "重温文化：探秘纸的逆袭",
  "introVideo": { "autoPlay": true, "url": "/assets/style-intro.mp4" },
  "video": { "url": "/assets/style-interact.mp4" },
  "breakpoints": {
    "quick-select": {
      "bp-qs-001": {
        "title": "思考时刻",
        "description": "15秒倒计时开始，动动手指，快速点击选择一个选项",
        "introBubbleText": "嗨，翻翻你的书包，如果我们现在还在用竹简书写字的话，如果1本书承载的信息需要1头驴来背，今天你需要牵几头驴来上学呢？",
        "timeLimit": 15,
        "selections": [
          { "id": "sel-1", "text": "3头驴" },
          { "id": "sel-2", "text": "5头驴" },
          { "id": "sel-3", "text": "8头驴" },
          { "id": "sel-4", "text": "10头驴" },
          { "id": "sel-5", "text": "更多驴" }
        ]
      }
    },
    "drag-sort": {
      "bp-ds-001": {
        "title": "思考时刻",
        "description": "读一读《纸的发明》第 4 自然段，将下面的造纸步骤按先后顺序拖拽排好。",
        "trailingText": "就成了一种即轻便又好用的纸。",
        "instruction": "操作说明：用手指长按下面要移动的词条，然后拖动到上面的框中",
        "blanks": [
          { "leadingText": "先" },
          { "leadingText": "然后" },
          { "leadingText": "再" },
          { "leadingText": "最后" }
        ],
        "entries": [
          { "id": "en-1", "text": "把树皮、麻头、破布、旧渔网剪碎或切断" },
          { "id": "en-2", "text": "浸在水里捣烂成浆" },
          { "id": "en-3", "text": "把浆捞出来" },
          { "id": "en-4", "text": "晒干" }
        ],
        "correctOrder": ["en-1", "en-2", "en-3", "en-4"],
        "maxErrors": 2
      }
    }
  }
}');

-- 27: 赵州桥 ZhaozhouBridgeParams
INSERT INTO node_contents (id, node_id, version, params_json) VALUES
(27, 20, 1, '{
  "title": "宣传有法：学习《赵州桥》的表达方法",
  "introVideo": { "autoPlay": false, "url": "/static/videos/zhaozhouqiao-intro.mp4" },
  "introBubbleText": "我们来梳理围绕一个意思把一段话写清楚的方法吧。",
  "cards": [
    {
      "id": "c1",
      "title": "在《赵州桥》的第 3 自然段里，一个意思指的是",
      "input": { "placeholder": "点击输入", "length": { "min": 1, "max": 50 } },
      "referenceAnswer": "美观",
      "maxErrors": 3
    },
    {
      "id": "c2",
      "title": "根据这一个意思写一句中心句：",
      "input": { "placeholder": "点击输入", "length": { "min": 1, "max": 100 } },
      "referenceAnswer": "这座桥不但坚固，而且美观",
      "maxErrors": 3
    },
    {
      "id": "c3",
      "title": "围绕这中心句，后面每一句话写的内容都跟这个意思有关。可以用上修辞手法，可以用事例或细节来写具体。请你读读中心句后面的句子，体会这种写法。",
      "voiceOnly": true,
      "maxErrors": 3
    },
    {
      "id": "c4",
      "title": "请再读一次，更好的去体会赵州桥的美观。",
      "voiceOnly": true,
      "maxErrors": 3,
      "assets": [
        { "type": "image", "src": "assets\\df4f8a70b6aa55f0fdafe45044e98dd43033e57b41ec8332218d8b4ab35c5cdd.gif" }
      ]
    }
  ]
}');

-- 28: 一幅名扬中外的画 WenMingZhongWaiParams（含 matrix 矩阵填空）
INSERT INTO node_contents (id, node_id, version, params_json) VALUES
(28, 21, 1, '{
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
}');

-- 29: 讲解优秀文化 HeritageCulturalParams（introBubbles 数组）
INSERT INTO node_contents (id, node_id, version, params_json) VALUES
(29, 22, 1, '{
  "string": "宣传文化：讲解优秀文化",
  "maxSubmissions": 3,
  "introVideo": { "autoPlay": false, "url": "/static/videos/heritage-intro.mp4" },
  "introBubbles": [
    { "role": "foreigner", "text": "刚才视频中介绍的内容我都非常感兴趣，可惜就是太简短了，你可以挑一个最熟悉的文化宝贝，再给我详细介绍一下吗？" },
    { "role": "native",    "text": "我们有这么多优秀的传统文化，你可以结合课文或者视频中的文化，也可以选一个自己熟悉的传统文化，来跟米娅说一说，说一段话就可以，但请围绕一个意思把这段话讲清楚。特别提醒：让你自豪的中华优秀传统文化会有很多方面，每一个方面都属于一个意思。" }
  ]
}');

-- 30: 创作工坊（节点级配置；任务级配置在 creation_tasks.config_json）
INSERT INTO node_contents (id, node_id, version, params_json) VALUES
(30, 23, 1, '{
  "title": "创作工坊",
  "introVideo": { "autoPlay": true, "url": "/static/videos/creation-ws-intro.mp4" },
  "introBubbleText": "用你喜欢的方式，创作一件与中华文化有关的作品吧！"
}');

-- ============== node_progress 学习进度（id 31-45） ==============
-- 学生 3/4/5 在部分节点完成学习；class_id 恒为 127
INSERT INTO node_progress (id, node_id, user_id, class_id, completed, attempt_count, error_count, revealed, draft_json, last_submit_id, duration) VALUES
-- 学生张小满(3)：写写感想已完成，初步感悟进行中
(31, 17, 3, 127, true,  3, 1, false, '{"inputs": {"q-wt-1": "我在《纸的发明》里最自豪的是蔡伦改进造纸术的智慧，他反复试验让纸又轻便又便宜，联系到我们现在写作业都用纸，觉得古人真了不起。"}}', NULL, 120),
(32, 18, 3, 127, false, 0, 0, false, '{}', NULL, 0),
-- 学生李思成(4)：写写感想、初步感悟均已完成；赵州桥进行中
(33, 17, 4, 127, true,  2, 0, false, '{"inputs": {"q-wt-1": "《赵州桥》让我自豪的是它的设计非常巧妙，桥下没有桥墩，却能承重千年，和我们现在的大桥一样坚固。"}}', NULL, 95),
(34, 18, 4, 127, true,  3, 2, false, '{"blanks": {"b-init-1": "赵州桥非常雄伟", "b-init-2": "这座桥不但坚固，而且美观"}}', NULL, 150),
(35, 20, 4, 127, false, 1, 1, false, '{}', NULL, 30),
-- 学生王语嫣(5)：文化风格节点完成
(36, 19, 5, 127, true,  4, 0, false, '{"quickSelect": {"bp-qs-001": "sel-3"}, "dragSort": {"bp-ds-001": ["en-1", "en-2", "en-3"]}}', NULL, 200),
(37, 22, 5, 127, false, 1, 0, false, '{"draft": "我来讲讲中国的剪纸艺术……"}', NULL, 45);

-- ============== node_submissions 提交明细（id 38-52） ==============
-- result_json 对应各 *SubmitResult / *SumbitResp schema
INSERT INTO node_submissions (id, code, class_id, node_id, user_id, node_type, question_id, submit_type, payload_json, result_json, is_processing, is_passed, is_completed, error_count, revealed, feedback, reference_answer, duration, points_earned) VALUES
-- 38-40 写写感想（学生4 三道题）
(38, 'sub-wt-001', 127, 17, 4, 'write-thoughts', 'q-wt-1', 'submit',
 '{"questionId": "q-wt-1", "text": "《赵州桥》让我自豪的是它的设计非常巧妙，桥下没有桥墩，却能承重千年。"}',
 '{"id": "sub-wt-001", "questionId": "q-wt-1", "type": "submit", "submittedText": "《赵州桥》让我自豪的是它的设计非常巧妙，桥下没有桥墩，却能承重千年。", "isProcessing": false, "isPassed": true, "isCompleted": true, "referenceAnswer": "围绕一个意思写出自豪的方面，并联系课文或生活。", "feedback": "棒极了！你抓住了赵州桥设计的巧妙之处，还联系了现代大桥，思路很清晰！", "offerReferenceAnswer": false}',
 false, true, true, 0, false, '棒极了！你抓住了赵州桥设计的巧妙之处！', '围绕一个意思写出自豪的方面，并联系课文或生活。', 60, 10),
(39, 'sub-wt-002', 127, 17, 4, 'write-thoughts', 'q-wt-2', 'submit',
 '{"questionId": "q-wt-2", "text": "《一幅名扬中外的画》让我自豪的是画家把一个平凡的生活场景画得那么热闹，每个人物都活灵活现。"}',
 '{"id": "sub-wt-002", "questionId": "q-wt-2", "type": "submit", "submittedText": "《一幅名扬中外的画》让我自豪的是画家把一个平凡的生活场景画得那么热闹，每个人物都活灵活现。", "isProcessing": false, "isPassed": false, "isCompleted": false, "feedback": "写出了热闹，但还没有点出这节课要学的表达方法哦，再试试？", "offerReferenceAnswer": true}',
 false, false, false, 1, false, '还没有点出表达方法，再试试？', NULL, 35, 0),
(40, 'sub-wt-003', 127, 17, 4, 'write-thoughts', 'q-wt-2', 'submit',
 '{"questionId": "q-wt-2", "text": "《一幅名扬中外的画》让我自豪的是作者围绕一个意思，抓住人物的不同神态把街市写清楚了。"}',
 '{"id": "sub-wt-003", "questionId": "q-wt-2", "type": "submit", "submittedText": "《一幅名扬中外的画》让我自豪的是作者围绕一个意思，抓住人物的不同神态把街市写清楚了。", "isProcessing": false, "isPassed": true, "isCompleted": true, "referenceAnswer": "围绕一个意思，抓住人物多种神态展开描写。", "feedback": "真棒！你不仅说出了自豪之处，还点明了围绕一个意思写清楚的方法！", "offerReferenceAnswer": false}',
 false, true, true, 1, false, '真棒！还点明了表达方法！', '围绕一个意思，抓住人物多种神态展开描写。', 40, 10),
-- 41-43 初步感悟（学生4 两道题，第二次correct）
(41, 'sub-init-001', 127, 18, 4, 'initial-insight', 'q-init-1', 'submit',
 '{"questionId": "q-init-1", "blankId": "b-init-1", "text": "赵州桥是世界上著名的石拱桥"}',
 '{"id": "sub-init-001", "questionId": "q-init-1", "blankId": "b-init-1", "submittedText": "赵州桥是世界上著名的石拱桥", "isProcessing": false, "isPassed": false, "isCompleted": false, "feedback": "再仔细读读第2自然段开头，想一想作者第一句在写什么？"}',
 false, false, false, 1, false, '再读读第2自然段开头。', NULL, 30, 0),
(42, 'sub-init-002', 127, 18, 4, 'initial-insight', 'q-init-1', 'submit',
 '{"questionId": "q-init-1", "blankId": "b-init-1", "text": "赵州桥非常雄伟"}',
 '{"id": "sub-init-002", "questionId": "q-init-1", "blankId": "b-init-1", "submittedText": "赵州桥非常雄伟", "isProcessing": false, "isPassed": true, "isCompleted": true, "referenceAnswer": "赵州桥非常雄伟", "feedback": "正确！这句就是第2自然段的中心意思。"}',
 false, true, true, 1, false, '正确！这就是中心意思。', '赵州桥非常雄伟', 25, 5),
(43, 'sub-init-003', 127, 18, 4, 'initial-insight', 'q-init-2', 'submit',
 '{"questionId": "q-init-2", "blankId": "b-init-2", "text": "这座桥不但坚固，而且美观"}',
 '{"id": "sub-init-003", "questionId": "q-init-2", "blankId": "b-init-2", "submittedText": "这座桥不但坚固，而且美观", "isProcessing": false, "isPassed": true, "isCompleted": true, "referenceAnswer": "这座桥不但坚固，而且美观", "feedback": "答对了！这句话承上启下，写出了桥的两个特点。"}',
 false, true, true, 0, false, '答对了！', '这座桥不但坚固，而且美观', 20, 5),
-- 44 文化风格 quick-select（学生5 正确）
(44, 'sub-cs-qs-001', 127, 19, 5, 'cultural-style', 'bp-qs-001', 'submit',
 '{"bpId": "bp-qs-001", "selected": "sel-3", "duration": 12}',
 '{"id": "sub-cs-qs-001", "bpId": "bp-qs-001", "selected": "sel-3", "isProcessing": false, "isPassed": true, "isCompleted": true, "referenceAnswer": "sel-3", "feedback": "回答正确！造纸术发明前，人们主要把字写在竹片和木片上。", "duration": 12}',
 false, true, true, 0, false, '回答正确！', 'sel-3', 12, 5),
-- 45 文化风格 drag-sort（学生5 第一次错误，46 第二次正确）
(45, 'sub-cs-ds-001', 127, 19, 5, 'cultural-style', 'bp-ds-001', 'submit',
 '{"bpId": "bp-ds-001", "answer": ["en-2", "en-1", "en-3"], "duration": 25}',
 '{"id": "sub-cs-ds-001", "bpId": "bp-ds-001", "answer": [{"id": "en-2", "text": "捣烂成浆"}, {"id": "en-1", "text": "剪碎切断"}, {"id": "en-3", "text": "晒干成纸"}], "isProcessing": false, "isPassed": false, "isCompleted": false, "feedback": "顺序不太对哦，先剪碎切断，再捣烂成浆。", "duration": 25}',
 false, false, false, 1, false, '顺序不太对哦。', NULL, 25, 0),
(46, 'sub-cs-ds-002', 127, 19, 5, 'cultural-style', 'bp-ds-001', 'submit',
 '{"bpId": "bp-ds-001", "answer": ["en-1", "en-2", "en-3"], "duration": 18}',
 '{"id": "sub-cs-ds-002", "bpId": "bp-ds-001", "answer": [{"id": "en-1", "text": "剪碎切断"}, {"id": "en-2", "text": "捣烂成浆"}, {"id": "en-3", "text": "晒干成纸"}], "isProcessing": false, "isPassed": true, "isCompleted": true, "referenceAnswer": [{"id": "en-1", "text": "剪碎切断"}, {"id": "en-2", "text": "捣烂成浆"}, {"id": "en-3", "text": "晒干成纸"}], "feedback": "完全正确！这就是蔡伦造纸的完整步骤。", "duration": 18}',
 false, true, true, 1, false, '完全正确！', NULL, 18, 5),
-- 47-48 赵州桥（学生4 朗读题 get-answer）
(47, 'sub-zzq-001', 127, 20, 4, 'zhaozhouqiao', 'zq-card-1', 'submit',
 '{"cardId": "zq-card-1", "text": "这座桥不但坚固，而且美观。", "duration": 45}',
 '{"id": "sub-zzq-001", "cardId": "zq-card-1", "type": "submit", "submittedText": "这座桥不但坚固，而且美观。", "isProcessing": false, "isPassed": true, "isCompleted": true, "referenceAnswer": "这座桥不但坚固，而且美观。", "feedback": "朗读流利，感情到位！", "duration": 45}',
 false, true, true, 0, false, '朗读流利！', '这座桥不但坚固，而且美观。', 45, 5),
(48, 'sub-zzq-002', 127, 20, 4, 'zhaozhouqiao', 'zq-card-2', 'submit',
 '{"cardId": "zq-card-2", "text": "", "audioResourceId": 53, "duration": 60}',
 '{"id": "sub-zzq-002", "cardId": "zq-card-2", "type": "submit", "submittedText": "", "submittedAudio": {"id": "res-53"}, "isProcessing": false, "isPassed": true, "isCompleted": true, "feedback": "发音准确，继续加油！", "duration": 60}',
 false, true, true, 0, false, '发音准确！', NULL, 60, 5),
-- 49 一幅名扬中外的画（学生3 矩阵填空，部分正确）
(49, 'sub-wmz-001', 127, 21, 3, 'wenmingzhongwai', 'wmz-r1', 'submit',
 '{"questionId": "wmz-r1", "answers": {"c1": "画上的街市可热闹了", "c2": "用了几百种不同的神态来表现人物"}}',
 '{"id": "sub-wmz-001", "answers": {"c1": "画上的街市可热闹了", "c2": "用了几百种不同的神态来表现人物"}, "isProcessing": false, "errors": [{"rowId": "r1", "blankId": "c1", "msg": "不正确，请重新输入"}], "isCompleted": false, "feedback": "第1空再看看中心句哦。"}',
 false, false, false, 1, false, '第1空再看看中心句。', NULL, 50, 0),
-- 50 讲解优秀文化（学生5 文本提交）
(50, 'sub-her-001', 127, 22, 5, 'heritage-cultural', NULL, 'submit',
 '{"text": "我来介绍春节：春节是中国人最重要的传统节日，一家人团聚吃年夜饭，贴春联、放鞭炮，象征着辞旧迎新。", "duration": 90}',
 '{"id": "sub-her-001", "submittedText": "我来介绍春节：春节是中国人最重要的传统节日，一家人团聚吃年夜饭，贴春联、放鞭炮，象征着辞旧迎新。", "isProcessing": false, "isPassed": true, "isCompleted": true, "referenceAnswer": "能围绕一个意思说清楚春节的一个方面并结合习俗展开。", "feedback": "讲解得很完整！围绕着团聚和辞旧迎新展开，条理清楚。", "duration": 90}',
 false, true, true, 0, false, '讲解得很完整！', '能围绕一个意思说清楚春节的一个方面。', 90, 10),
-- 51 get-answer 类型提交（写写感想，学生3 查看答案）
(51, 'sub-wt-get-ans', 127, 17, 3, 'write-thoughts', 'q-wt-1', 'get-answer',
 '{"questionId": "q-wt-1", "type": "get-answer"}',
 '{"id": "sub-wt-get-ans", "questionId": "q-wt-1", "type": "get-answer", "submittedText": null, "isProcessing": false, "isPassed": false, "isCompleted": true, "referenceAnswer": "围绕一个意思写出自豪的方面，并联系课文或生活。", "feedback": "这是本题的参考答案，下次加油自己写出来！", "offerReferenceAnswer": false}',
 false, false, true, 0, true, '这是本题的参考答案。', '围绕一个意思写出自豪的方面。', 10, 0),
-- 52 创作工坊（学生3 表达类提交）
(52, 'sub-cw-expr-001', 127, 23, 3, 'creation-workshop', NULL, 'submit',
 '{"taskType": "express", "textContent": "我要介绍京剧：京剧是中国的国粹，脸谱颜色各有含义，红脸代表忠勇，黑脸代表刚正。"}',
 '{"id": "sub-cw-expr-001", "taskType": "express", "textContent": "我要介绍京剧：京剧是中国的国粹……", "isProcessing": false, "isPassed": true, "isCompleted": true, "feedback": "介绍得很清楚，还点出了脸谱颜色的含义！", "duration": 80}',
 false, true, true, 0, false, '介绍得很清楚！', NULL, 80, 10);

-- ============== resources 文件资源（id 53-56） ==============
INSERT INTO resources (id, resource_id, media_type, file_name, file_url, resource_suffix, resource_size, resource_info, resource_source, upload_oss, image_compress, video_compress, owner_id) VALUES
(53, 'res-53', 'audio', 'zhao_zhou_qiao_reading.wav',  '/static/audio/zhao_zhou_qiao_reading.wav',  'wav',  204800, '赵州桥朗读录音（测试）', 'student_upload', true, false, false, 4),
(54, 'res-54', 'image', 'paper_invention.png',        '/static/images/paper_invention.png',       'png',  512000, '纸的发明课件图',      'resource_library', true, true, false, 2),
(55, 'res-55', 'image', 'zhao_zhou_qiao_bridge.png',  '/static/images/zhao_zhou_qiao_bridge.png', 'png',  768000, '赵州桥实景图',        'resource_library', true, true, false, 2),
(56, 'res-56', 'video', 'style_interact.mp4',         '/static/videos/style_interact.mp4',         'mp4', 5242880,'互动视频文件',        'resource_library', true, false, true, 2);

-- ============== audio_transcriptions 语音转写（id 57，关联资源53） ==============
INSERT INTO audio_transcriptions (id, resource_id, status, text) VALUES
(57, 53, 'finish', '这座桥不但坚固，而且美观。');

-- ============== creation_tasks 创作工坊任务（id 58-61） ==============
INSERT INTO creation_tasks (id, code, node_id, task_type, title, config_json) VALUES
(58, 'task-gen-001',    23, 'generation',  '手抄报/海报创作',
 '{"introBubbleText": "设计一张介绍中华文化的手抄报", "title": "手抄报与海报", "description": "输入画面描述与文字内容，AI 帮你生成海报图片", "inputs": [{"id": "scene-desc", "placeholder": "画面描述，如：张衡地动仪手抄报画面描述"}, {"id": "text-content", "placeholder": "手抄报内展示文字"}]}'),
(59, 'task-poem-001',   23, 'poemscripts', '古诗创作',
 '{"introBubbleText": "发挥想象，创作一首小诗", "title": "古诗创作", "description": "围绕中华文化主题创作五言或七言古诗", "inputs": [{"id": "poem-content", "placeholder": "输入你的诗作"}]}'),
(60, 'task-share-001',  23, 'share',       '创作分享',
 '{"introBubbleText": "把你的作品分享给全班同学", "title": "创作分享", "description": "发布你的作品并听取大家的评价", "inputs": [{"id": "share-content", "placeholder": "输入分享说明"}]}'),
(61, 'task-expr-001',   23, 'express',     '表达/讲解',
 '{"introBubbleText": "讲一讲你了解的一项传统文化", "title": "表达讲解", "description": "围绕一个意思，把你想介绍的传统文化说清楚", "inputs": [{"id": "express-content", "placeholder": "输入你的讲解内容"}]}');

-- ============== creation_submissions 工坊提交（id 62-64） ==============
INSERT INTO creation_submissions (id, code, task_id, node_id, user_id, class_id, task_type, text_content, content_json, ai_image_resource_id, ai_feedback, ai_revised_content, ai_evaluation_json, status, is_processing, is_successful, is_completed, is_published, duration, points_earned, submitted_at, published_at) VALUES
-- 62 手抄报/海报生成成功未发布
(62, 'cd-sub-gen-001', 58, 23, 5, 127, 'generation', '张衡地动仪手抄报',
 '{"sceneDesc": "张衡地动仪手抄报画面描述", "textContent": "手抄报内展示文字：地动仪是中国古代科技的骄傲"}',
 55,
 '海报图片已生成，画面完整，色彩和谐。', NULL,
 '{"id": "cd-sub-gen-001", "isProcessing": false, "isSuccessful": true, "isCompleted": true, "feedback": "海报图片已生成！", "content": {"sceneDesc": "张衡地动仪手抄报画面描述"}, "AIimage": {"id": "res-55", "src": "/static/images/zhao_zhou_qiao_bridge.png"}, "isPublish": false, "duration": 30, "taskType": "generation"}',
 'successful', false, true, true, false, 30, 10, now() - interval '2 hours', NULL),
-- 63 古诗创作成功
(63, 'cd-sub-poem-001', 59, 23, 4, 127, 'poemscripts', '青山绿水映朝阳，中华文化万年长。',
 '{"taskType": "ancient_poem_7", "textContent": "青山绿水映朝阳，中华文化万年长。"}',
 NULL,
 '诗作押韵工整，意境优美！', '青山映水水映天，中华文脉万千年。',
 '{"id": "cd-sub-poem-001", "isProcessing": false, "isSuccessful": true, "isCompleted": true, "feedback": "诗作押韵工整，意境优美！", "TextContent": {"textContent": "青山绿水映朝阳，中华文化万年长。"}, "duration": 60, "evaluation": {"text": "押韵工整，意境优美", "audio": null}}',
 'successful', false, true, true, false, 60, 10, now() - interval '1 day', NULL),
-- 64 分享已发布
(64, 'cd-sub-share-001', 60, 23, 3, 127, 'share', '这是我为春节设计的手抄报，希望大家喜欢！',
 '{"content": "这是我为春节设计的手抄报，希望大家喜欢！", "imageUrl": "/static/images/paper_invention.png"}',
 54,
 '分享成功！', NULL,
 '{"id": "cd-sub-share-001", "isProcessing": false, "isSuccessful": true, "isCompleted": true, "feedback": "分享成功！", "content": {"content": "这是我为春节设计的手抄报"}, "AIimage": {"id": "res-54", "src": "/static/images/paper_invention.png"}, "isPublish": true, "duration": 20, "taskType": "share"}',
 'published', false, true, true, true, 20, 5, now() - interval '3 days', now() - interval '3 days');

-- ============== moments 朋友圈动态（id 65-66） ==============
INSERT INTO moments (id, class_id, user_id, source, content, objs_json, like_count, comment_count) VALUES
(65, 127, 3, '意象词云', '这是我为春节设计的手抄报，祝大家新年快乐！',
 '[{"type": "v4-write-reflections", "obj": [{"title": "在《纸的发明》中，是哪些让你感到自豪？", "content": "蔡伦改进造纸术的智慧让我自豪。", "isPassed": true}]}]',
 2, 1),
(66, 127, 4, '创作工坊', '我的诗作分享：青山绿水映朝阳，中华文化万年长。',
 '[{"type": "v4-answer-with-reference", "obj": {"answer": "青山绿水映朝阳，中华文化万年长。", "reference": "青山映水水映天，中华文脉万千年。", "layoutDirection": "vertical"}}]',
 1, 2);

-- ============== moment_images 朋友圈图片（id 67-68） ==============
INSERT INTO moment_images (id, moment_id, resource_id, sort_order) VALUES
(67, 65, 54, 0),
(68, 66, 55, 0);

-- ============== moment_likes 朋友圈点赞（id 69-71） ==============
INSERT INTO moment_likes (id, moment_id, user_id) VALUES
(69, 65, 4),
(70, 65, 5),
(71, 66, 3);

-- ============== moment_comments 朋友圈评论（id 72-74） ==============
INSERT INTO moment_comments (id, moment_id, user_id, content) VALUES
(72, 65, 4, '手抄报做得真漂亮！'),
(73, 66, 3, '诗写得太好了，很有意境！'),
(74, 66, 5, '下次教教我写诗吧！');

-- ============== ai_sessions / ai_messages / ai_search_references（id 75+） ==============
INSERT INTO ai_sessions (id, code, user_id, class_id, node_id, biz_type, title, prompt_template, status) VALUES
(75, 'sess-001', 3, 127, 17, 'write-thoughts', '关于《纸的发明》的讨论', '你是引导学生思考的语文老师，善于启发式提问。', 'active');

INSERT INTO ai_messages (id, session_id, role, content, finish_reason, prompt_tokens, completion_tokens, total_tokens, model_name, seq) VALUES
(76, 75, 'user',      '为什么说蔡伦改进造纸术很了不起？', 'stop',  120,  0,  120, 'qwen-max', 1),
(77, 75, 'assistant', '因为蔡伦用树皮、麻头等原料造出了又轻便又便宜的纸，让更多人用上了纸，这是了不起的智慧！', 'stop', 0, 200, 200, 'qwen-max', 2),
(78, 75, 'user',      '谢谢老师，我明白了！',           'stop',  80,  0,   80, 'qwen-max', 3);

INSERT INTO ai_search_references (id, message_id, ref_index, title, link, snippet) VALUES
(79, 77, 0, '蔡伦改进造纸术', 'https://example.com/zhishi/cailun', '蔡伦总结前人的经验，用树皮、麻头、稻草、破布等原料造纸。');

-- 数据校验提示
DO $$
DECLARE
    _max_id bigint;
BEGIN
    SELECT max(id) INTO _max_id FROM (
        SELECT max(id) AS id FROM users
        UNION ALL SELECT max(id) FROM class_members
        UNION ALL SELECT max(id) FROM class_nodes
        UNION ALL SELECT max(id) FROM node_contents
        UNION ALL SELECT max(id) FROM node_progress
        UNION ALL SELECT max(id) FROM node_submissions
        UNION ALL SELECT max(id) FROM resources
        UNION ALL SELECT max(id) FROM audio_transcriptions
        UNION ALL SELECT max(id) FROM creation_tasks
        UNION ALL SELECT max(id) FROM creation_submissions
        UNION ALL SELECT max(id) FROM moments
        UNION ALL SELECT max(id) FROM moment_images
        UNION ALL SELECT max(id) FROM moment_likes
        UNION ALL SELECT max(id) FROM moment_comments
        UNION ALL SELECT max(id) FROM ai_sessions
        UNION ALL SELECT max(id) FROM ai_messages
        UNION ALL SELECT max(id) FROM ai_search_references
    ) t;
    RAISE NOTICE 'max global id = %', _max_id;
END $$;

COMMIT;