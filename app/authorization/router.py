from sqlalchemy.exc import IntegrityError
from fastapi import APIRouter, Depends, Response, status
from jose import jwt, JWTError
from datetime import datetime

from app.users.services import UserService
from app.config import settings
from app.users.schemas import UserCreate, UserLogin, User as UserSchema
from app.authorization.dependencies import check_is_current_user_root, get_token
from app.authorization.authorization import (
    get_password_hash,
    authenticate_user,
    create_access_token,
)
from app.exceptions import (
    IncorrectFormatTokenException,
    TokenExpiredException,
    UserAlreadyExistsException,
    IncorrectEmailOrPasswordException,
    UniquePhoneNumberException,
    UserIsNotPresentException,
    UserIsBannedException
)


router = APIRouter(
    prefix="/authorization",
    tags=["Аутентификация"],
)


@router.post("/register", response_model=dict, status_code=status.HTTP_201_CREATED)
async def register_user(user_data: UserCreate, current_user: UserSchema = Depends(check_is_current_user_root)) -> dict:
    """
    Регистрация пользователя
    """
    existing_user = await UserService.find_one_or_none(email=user_data.email)
    if existing_user:
        raise UserAlreadyExistsException

    hashed_password = get_password_hash(user_data.password)
    try:
        await UserService.add(
            email=user_data.email,
            password=hashed_password,
            first_name=user_data.first_name,
            last_name=user_data.last_name,
            paternal_name=user_data.paternal_name,
            phone_number=user_data.phone_number,
        )
    except IntegrityError:
        raise UniquePhoneNumberException
    
    return {"message": f"Пользователь '{user_data.first_name} {user_data.last_name}' успешно создан"}


@router.post("/login", response_model=dict, status_code=status.HTTP_200_OK)
async def login_user(response: Response, user_data: UserLogin) -> dict:
    """
    Авторизация пользователя
    """
    user = await authenticate_user(user_data.email, user_data.password)
    if not user:
        raise IncorrectEmailOrPasswordException

    access_token = create_access_token({"sub": str(user.id)})
    response.set_cookie('access_token', access_token, httponly=True)

    return {"access_token": access_token}


@router.post("/logout", response_model=dict, status_code=status.HTTP_200_OK)
async def logout_user(response: Response) -> dict:
    """
    Выход пользователя
    """
    response.delete_cookie("access_token")
    return {"message": "До свидания!"}


@router.post("/valid_check", status_code=status.HTTP_200_OK)
async def access_token_valid_check(token = Depends(get_token)):
    if token:
        try:
            payload = jwt.decode(token, settings.SECRET_KEY, settings.ALGORITHM)
        except JWTError:
            raise IncorrectFormatTokenException

        expire: str = payload.get("exp")
        if not expire or datetime.utcnow().timestamp() > expire:
            raise TokenExpiredException

        user_id: str = payload.get("sub")
        if not user_id:
            raise UserIsNotPresentException 

        user = await UserService.find_by_id(user_id)
        if not user:
            raise UserIsNotPresentException  
        
        if user.ban == True:
            raise UserIsBannedException

        return {"user": user}
    return{"detail": "false"}
