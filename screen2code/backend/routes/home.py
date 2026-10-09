from fastapi import APIRouter
from fastapi.responses import HTMLResponse

router = APIRouter()


@router.get("/")
async def get_status():
    return HTMLResponse(
        content="<h3>后端运行正常。请打开前端地址（默认 http://localhost:5173）使用 AI 一键复刻网站。</h3>"
    )
