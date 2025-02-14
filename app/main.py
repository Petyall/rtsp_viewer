import time, traceback

from fastapi import FastAPI, Request
from fastapi.staticfiles import StaticFiles
from fastapi.middleware.cors import CORSMiddleware

from app.users.router import router as users_router
from app.stream.router import router as stream_router
from app.cameras.router import router as cameras_router
from app.authorization.router import router as authorization_router
from app.importer.router import router as importer_router
from app.logger import logger
from app.config import settings


app = FastAPI()

app.include_router(authorization_router)
app.include_router(users_router)
app.include_router(cameras_router)
app.include_router(stream_router)
app.include_router(importer_router)

# app.mount("/streams", StaticFiles(directory=settings.STREAMS_DIR), name="streams")

origins = ["*"]

app.add_middleware(
    CORSMiddleware,
    allow_origins=origins,
    allow_credentials=True,
    allow_methods=["GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"],
    allow_headers=["Content-Type", "Authorization", "Set-Cookie", "Access-Control-Allow-Origin", "Access-Control-Allow-Headers"],
)

# @app.middleware("http")
# async def log_requests(request: Request, call_next):
#     start_time = time.time()
    
#     try:
#         logger.info(f"Started request: {request.method} {request.url} from {request.client.host}")
#         response = await call_next(request)
#         process_time = time.time() - start_time
#         logger.info(f"Ended request: {request.method} {request.url} in {round(process_time, 4)} second")
#         return response
#     except Exception as exc:
#         logger.error(f"Request error {request.method} {request.url}: {str(exc)}")
#         logger.error(traceback.format_exc())
#         raise exc

# @router.get("/streams/{camera_id}/{filename}")
# async def get_hls_segment(camera_id: int, filename: str, current_user: User = Depends(get_current_user)):
#     raise HTTPException(status_code=404, detail="Запрос на сегмент HLS не поддерживается напрямую через FastAPI")
