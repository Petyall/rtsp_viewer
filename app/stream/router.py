import httpx

from app.config import settings
from app.users.schemas import User as UserSchema
from app.authorization.dependencies import get_current_user, get_token
from app.cameras.services import CameraService, UserCameraService
from app.exceptions import CameraNotFoundException, UserCameraNotFoundException

from fastapi.templating import Jinja2Templates
from fastapi import APIRouter, Depends, status, HTTPException, Request


router = APIRouter(
    prefix="/stream",
    tags=["Работа с потоком"],
)

templates = Jinja2Templates(directory="./app/templates")
GIN_HOST = settings.GIN_HOST


@router.get("/start/{camera_id}", status_code=status.HTTP_200_OK)
async def stream_camera(camera_id: int, current_user: UserSchema = Depends(get_current_user), token: str = Depends(get_token)):
    """
    Возвращает URL .m3u8 для потока камеры, если у пользователя есть доступ.
    """
    user_camera = await UserCameraService.find_one_or_none(user_id=current_user.id, camera_id=camera_id)
    if not user_camera:
        raise UserCameraNotFoundException

    camera = await CameraService.find_one_or_none(id=camera_id)
    if not camera:
        raise CameraNotFoundException

    try:
        async with httpx.AsyncClient(timeout=httpx.Timeout(30.0)) as client:
            response = await client.post(
                f"{GIN_HOST}/start/{camera_id}",
                headers={"Authorization": f"Bearer {token}"}
            )
            response.raise_for_status()
    except httpx.HTTPStatusError as e:
        raise HTTPException(status_code=e.response.status_code, detail="Не удалось запустить поток")

    stream_url = f"{GIN_HOST}/streams/camera_{camera_id}/index.m3u8"
    return {"stream_url": stream_url}


# @router.get("/start/{camera_id}", status_code=status.HTTP_200_OK)
# async def stream_camera(request: Request, camera_id: int, current_user: UserSchema = Depends(get_current_user), token: str = Depends(get_token)):
#     """
#     Потоковое воспроизведение камеры, к которой у пользователя есть доступ
#     """
#     user_camera = await UserCameraService.find_one_or_none(user_id=current_user.id, camera_id=camera_id)
#     if not user_camera:
#         raise UserCameraNotFoundException

#     camera = await CameraService.find_one_or_none(id=camera_id)
#     if not camera:
#         raise CameraNotFoundException
#     print(f"Отправляемый токен: Bearer {token}")

#     try:
#         async with httpx.AsyncClient(timeout=httpx.Timeout(30.0)) as client:
#             response = await client.post(
#                 f"{GIN_HOST}/start/{camera_id}",
#                 headers={"Authorization": f"Bearer {token}"}
#             )
#             response.raise_for_status()
#     except httpx.HTTPStatusError as e:
#         print(HTTPException(status_code=e.response.status_code, detail="Не удалось запустить поток"))

#     return templates.TemplateResponse("index.html", {"request": request})


@router.get("/stop/{camera_id}", status_code=status.HTTP_200_OK)
async def stream_camera_stop(request: Request, camera_id: int, current_user: UserSchema = Depends(get_current_user), token: str = Depends(get_token)):
    user_camera = await UserCameraService.find_one_or_none(user_id=current_user.id, camera_id=camera_id)
    if not user_camera:
        raise UserCameraNotFoundException

    camera = await CameraService.find_one_or_none(id=camera_id)
    if not camera:
        raise CameraNotFoundException

    try:
        async with httpx.AsyncClient(timeout=httpx.Timeout(15.0)) as client:
            response = await client.post(
                f"{GIN_HOST}/stop/{camera_id}",
                headers={"Authorization": f"Bearer {token}"}
            )
            response.raise_for_status()
    except httpx.HTTPStatusError as e:
        print(HTTPException(status_code=e.response.status_code, detail="Не удалось остановить поток"))

    return templates.TemplateResponse("index.html", {"request": request})

# # Костыль
# @router.get("/start/{camera_id}", status_code=status.HTTP_200_OK)
# async def stream_camera(request: Request, camera_id: int):
# # async def stream_camera(request: Request, camera_id: int, current_user: UserSchema = Depends(get_current_user), token: str = Depends(get_token)):
#     """
#     Потоковое воспроизведение камеры, к которой у пользователя есть доступ
#     """
#     token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiI4ZWY5ZjJhZi1hMWMxLTQwMDktOTMwNi0xMTgxNzRiM2U5NmYiLCJleHAiOjE3MzM3NTMwNDB9.GYeWgz8PqtvDKhjPUh2hSMS3uESzNykJqf-_y2s4whQ"
#     user_camera = await UserCameraService.find_one_or_none(user_id="8ef9f2af-a1c1-4009-9306-118174b3e96f", camera_id=camera_id)
#     if not user_camera:
#         raise UserCameraNotFoundException

#     camera = await CameraService.find_one_or_none(id=camera_id)
#     if not camera:
#         raise CameraNotFoundException
#     print(f"Отправляемый токен: Bearer {token}")

#     try:
#         async with httpx.AsyncClient(timeout=httpx.Timeout(15.0)) as client:
#             response = await client.post(
#                 f"{GIN_HOST}/start/{camera_id}",
#                 headers={"Authorization": f"Bearer {token}"}
#             )
#             response.raise_for_status()
#     except httpx.HTTPStatusError as e:
#         print(HTTPException(status_code=e.response.status_code, detail="Не удалось запустить поток"))

#     return templates.TemplateResponse("index.html", {"request": request})