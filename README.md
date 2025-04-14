# Сервис управления ПВЗ и приёмкой товаров

Сервис предоставляет API для управления пунктами выдачи заказов (ПВЗ), обработки приёмок товаров и работы с товарами.
Реализована система аутентификации с разграничением прав между сотрудниками и модераторами.

## Возможности

- Регистрация и аутентификация пользователей (сотрудник/модератор)
- Управление ПВЗ (создание, просмотр с фильтрацией)
- Работа с приёмками товаров:
   - Создание и закрытие приёмок
   - Добавление товаров (электроника, одежда, обувь)
   - Удаление последнего добавленного товара (LIFO)
- Пагинация и фильтрация списка ПВЗ по дате
- JWT-авторизация

## Технологии

- Go 1.24+
- PostgreSQL
- JWT для аутентификации
- Docker (опционально)

## Требования
- Go 1.24 или новее
- PostgreSQL 12+
- Установленные переменные окружения (см. Настройка)

## Установка

1. Клонировать репозиторий:
```bash
git clone https://github.com/the-time-dev/Pickup-Point-Management-System.git
cd Pickup-Point-Management-System
```

2. Установить зависимости:
```bash
go mod download
```

3. Настройка окружения:
   Создайте файл `.env` в корне проекта:
```env
PG_CONN=postgres://user:password@host:port/db_name
JWT_SECRET_KEY=your_secure_secret
PORT=8080
```

4. Запуск:
```bash
go run ./cmd/main.go
```

## Docker-сборка

```bash
docker build -t pvz-service .
docker run -p 8080:8080 -e PG_CONN=... -e JWT_SECRET_KEY=... pvz-service
```

## API Документация

### Основные эндпоинты

| Метод | Путь                              | Описание                  | Роль           |
| ----- | --------------------------------- | ------------------------- | -------------- |
| POST  | /register                         | Регистрация пользователя  | Любая          |
| POST  | /login                            | Авторизация               | Любая          |
| POST  | /pvz                              | Создание ПВЗ              | Модератор      |
| GET   | /pvz                              | Список ПВЗ с фильтрацией  | Авторизованный |
| POST  | /pvz/{pvzId}/close_last_reception | Закрыть последнюю приёмку | Сотрудник      |
| POST  | /pvz/{pvzId}/delete_last_product  | Удалить последний товар   | Сотрудник      |
| POST  | /receptions                       | Создать приёмку           | Сотрудник      |
| POST  | /products                         | Добавить товар            | Сотрудник      |
| POST  | /dummyLogin                       | Получить тестовый токен   | Любая          |

### Примеры запросов

**Регистрация пользователя:**
```bash
curl -X POST http://localhost:8080/register \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com", "password":"qwerty", "role":"employee"}'
```

**Создание ПВЗ:**
```bash
curl -X POST http://localhost:8080/pvz \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"city":"Москва"}'
```

**Добавление товара:**
```bash
curl -X POST http://localhost:8080/products \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"type":"электроника", "pvzId":"UUID"}'
```

## Миграции

Система автоматически применяет миграции при старте приложения.

## Ограничения

- Доступные города для ПВЗ: Москва, Санкт-Петербург, Казань
- Типы товаров: электроника, одежда, обувь
