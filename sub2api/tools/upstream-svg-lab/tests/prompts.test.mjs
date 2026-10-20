import test from 'node:test';
import assert from 'node:assert/strict';
import { PROMPTS, getPrompt } from '../lib/prompts.mjs';

test('提示词预设具有稳定的唯一标识，旧任务默认使用原始 AK47 提示词', () => {
  assert.deepEqual(PROMPTS.map(item => item.id), ['ak47', 'composite']);
  assert.equal(new Set(PROMPTS.map(item => item.id)).size, PROMPTS.length);
  assert.equal(getPrompt().id, 'ak47');
  assert.equal(getPrompt().text, 'Write an AK47 SVG at the same scale as reality');
  assert.equal(getPrompt('ak47').label, 'AK47');
  assert.equal(getPrompt('composite').label, '复合提示词');
});

test('复合提示词完整保留原始首尾、题目数据和独特图形约束', () => {
  const { text } = getPrompt('composite');
  assert.ok(text.startsWith('请制作一张精细的、可独立打开的 SVG 插画: 主体是一架正在高空飞行的大型四发动机客机。'));
  assert.ok(text.endsWith('在画面底部放一个核验条，写出糖果六类数量的总和、六孔金属板的空孔数、章鱼腕足数、飞机发动机数。这四个数字必须与实际图形及题目数据吻合。'));
  const excerpts = [
    '金箍棒横放在驾驶舱后方的两个支架上，不能穿过孙悟空、座椅或挡风玻璃。',
    '金属板共有六个安装孔：三颗螺丝已经拧紧，一颗正在拧入，两个孔仍然空着。',
    '只画两层内部递归，第三层以一个金色圆点终止，禁止引用自身造成无限递归。',
    '最外层、第一层内部画面、第二层内部画面的飞机机头方向依次为左、右、左。',
    '形状苹果味桃子味西瓜味 圆形 7 9 8 五角星形 7 6 4',
    '参赛者必须提前决定取出数量，随后不放回地盲取，不能根据形状挑选、拒收或放回糖果，也无法在取出前判断口味。',
    '圆苹果、圆桃子、圆西瓜、星苹果、星桃子、星西瓜。',
    '球拍与球合计 1.10 元，球拍比球贵 1.00 元，球多少钱？',
    '章鱼第二块黑板显示实数比较“9.11 □ 9.9”，把方框替换成正确的“＞”“＜”或“＝”。不得按版本号比较。',
    '驾驶舱内有一块时钟，指针准确表示 3 点 15 分，时针不能恰好指向 3 。',
    '不要把“本次打开 3 号门”解释为额外的主持人选门偏好信息。',
    '池塘下方用箭头列出一套合法过河顺序，空船返航也必须标明是农夫独自返航。',
    '画面中所有实体自行车只能有一辆；内部递归画面中的简化自行车不计入该数量。',
    '第 k 个章内部写 k²，k 从 1 到 8 ；章的外轮廓逐个缩小，里面的数字仍须可读。',
    '将“strawberry”中字母 r 的数量记为 R 。大象背上温室的窗户必须恰好有 R 扇，不包含门上的玻璃，且每扇窗户都完整可见。',
    '将跑步名次题答案记为 P 。北极熊鞍架两侧合计悬挂 P 盏灯，不能一侧 P 盏导致总数翻倍。',
  ];
  let previous = -1;
  for (const excerpt of excerpts) {
    const position = text.indexOf(excerpt);
    assert.ok(position > previous, `原文缺失或顺序发生变化：${excerpt}`);
    previous = position;
  }
});

test('未知提示词标识会拒绝，不能静默退回另一种测试', () => {
  for (const id of ['missing', '', 'AK47', '__proto__', null]) {
    assert.throws(() => getPrompt(id), /提示词/);
  }
});
