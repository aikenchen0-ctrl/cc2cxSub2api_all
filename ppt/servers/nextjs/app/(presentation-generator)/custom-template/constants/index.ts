/**
 * Constants for Custom Template Creation Flow
 */

import { TemplateCreationStep } from "../types";

// Step configuration
export const TEMPLATE_STEPS: Record<TemplateCreationStep, { title: string; description: string }> = {
    'file-upload': {
        title: '上传模板',
        description: '上传您的 PPTX 文件开始',
    },
    'font-check': {
        title: '字体检查',
        description: '正在检查演示文稿中的字体',
    },
    'font-upload': {
        title: '上传字体',
        description: '上传缺失字体以确保准确渲染',
    },
    'slides-preview': {
        title: '预览幻灯片',
        description: '处理前请检查您的幻灯片',
    },
    'template-creation': {
        title: '模板创建',
        description: '将幻灯片转换为可重用的模板',
    },
    'completed': {
        title: '已完成',
        description: '您的模板已准备就绪，可以保存',
    },
};

// UI Configuration
export const UI_CONFIG = {
    schemaEditorWidth: '520px',
    slideGridGap: '20px',
    maxContentWidth: '1400px',
}
// Highlights for benefits section
export const HIGHLIGHTS_ITEMS = [
    {
        number: "1",
        title: "耗时",
        description: "手动排版和幻灯片复制每周浪费数小时",
    },
    {
        number: "2",
        title: "昂贵",
        description: "将设计精力从重复性任务转向创新",
    },
    {
        number: "3",
        title: "不一致",
        description: "AI 生成不可预测的布局，需要不断清理",
    },
]

export const FAQS = [
    {
        question: "What is Custom Template Creation?",
        answer: "Custom Template Creation is a feature that allows you to create custom templates for your presentations.",
    },
    {
        question: "How do I create a custom template?",
        answer: "You can create a custom template by uploading a PPTX file and then editing the template to your liking.",
    },
    {
        question: "How do I edit a custom template?",
        answer: "You can edit a custom template by uploading a PPTX file and then editing the template to your liking.",
    },
    {
        question: "How do I delete a custom template?",
        answer: "You can delete a custom template by uploading a PPTX file and then editing the template to your liking.",
    },
    {
        question: "How do I create a custom template?",
        answer: "You can create a custom template by uploading a PPTX file and then editing the template to your liking.",
    },
    {
        question: "How do I edit a custom template?",
        answer: "You can edit a custom template by uploading a PPTX file and then editing the template to your liking.",
    },
    {
        question: "How do I delete a custom template?",
        answer: "You can delete a custom template by uploading a PPTX file and then editing the template to your liking.",
    },
]
