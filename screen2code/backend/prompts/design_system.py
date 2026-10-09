def build_design_system_prompt_block(design_system: str | None) -> str:
    if not design_system or not design_system.strip():
        return ""

    return f"""
## 设计规范

设计规范与其他要求冲突时，优先遵循设计规范。

<design_system>
{design_system.strip()}
</design_system>
"""
