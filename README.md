[![License: MPL 2.0](https://img.shields.io/badge/License-MPL_2.0-brightgreen.svg)](https://opensource.org/licenses/MPL-2.0)
[![en](https://img.shields.io/badge/README-en-green.svg)](https://github.com/aisa-it/aiplan/blob/main/README.md)
[![ru](https://img.shields.io/badge/README-ru-green.svg)](https://github.com/aisa-it/aiplan/blob/main/README.ru.md)
[![telegram](https://img.shields.io/badge/Telegram-2CA5E0?style=for-the-badge&logo=telegram&logoColor=white)](https://t.me/aiplan_faq)
# AIPlan - An open-source project management system
Get to know AIPlan, a professional project management platform that helps teams track tasks, make phone calls, maintain documentation, and produce a high—quality product.
The product provides convenient tools for planning, tracking, and completing tasks, as well as for teamwork within a team.
The system is designed for small, medium-sized teams and for large organizations that seek to optimize work processes, improve communication and increase productivity.

You can easily transfer your work from Jira, as the **AIPlan** implements simple and convenient **project import from Jira**. The built-in **AIDoc** documentation management system offers **an analog of Confluence** with a convenient editor and the ability to collaborate on documents.

The AIPLAN is constantly being improved. Your suggestions and bug reports help us to become better. Create support requests. https://t.me/aiplan_faq .

## 🌟 Key features
1. **Task Management**:
   - Transfer all your tasks from Jira in a few clicks.
   - Create tasks with deadlines, priorities, and responsible persons.
   - The ability to link tasks to each other for more detailed planning.
   - Assigning roles and access rights to project participants.
   - Real-time task completion status tracking.
2. **Working on documents**:
   - The ability to create and store documents directly in the system.
   - A convenient text editor for editing documents.
   - Organize documents into folders and projects for easy access.
3. **Forms**:
   - Create forms to collect feedback or data.
   - Generation of unique links to forms that can be sent to users (both within the team and to external participants).
   - Automatic saving of the received data in the system.
4. **Calendar**:
   - An interactive calendar for viewing all tasks and activities by day, week or month.
   - The ability to filter tasks by responsible persons, projects or statuses.
5. **Video Calls and Conferences**:
   - Built-in video calling tool directly in the system.
   - The ability to organize meetings with team members without the need to use third-party applications.
   - Screen interaction support: screen demonstration during calls.
   - Record meetings for later viewing or analysis.
6. **Integration**:
   - The ability to export data in formats .docx, .pdf.
   - The ability to import data from Jira.
7. **Reports and analytics**:
   - Tracking user activity on projects.
8. **Notifications and Reminders**:
   - Customizable notifications about new tasks and any changes to them.
   - Support for notifications via email, Telegram and within the system.

## 📝 Documentation
If you have any questions and for a detailed study of the product's capabilities, you can always refer to the User's Manual, which you will find inside the product.

## How to install
The configuration is done by .env file.
```
docker-compose up -d
```
- The application will be available at http://localhost:8080
- Default user (superuser) email: `DEFAULT_EMAIL`; password: `password123`

## Application Parameters

| Parameter                     | Description                                                                | Type   |
|-------------------------------|----------------------------------------------------------------------------| ------ |
| `SECRET_KEY`                  | The key for generating JWT tokens.                                         | string |
| `AWS_REGION`                  | Minio region                                                               | string |
| `AWS_ACCESS_KEY_ID`           | minio login                                                                | string |
| `AWS_SECRET_ACCESS_KEY`       | minio password                                                             | string |
| `AWS_S3_ENDPOINT_URL`         | Path to minio                                                              | string |
| `AWS_S3_BUCKET_NAME`          | Name of the minio bucket                                                   | string |
| `DATABASE_URL`                | DSN of the database                                                        | string |
| `DEFAULT_EMAIL`               | Email of the standard user (password `password123` at creation)            | string |
| `EMAIL_ACTIVITY_DISABLED`     | Disabling sending notifications to                                         | bool   |
| `EMAIL_HOST`                  | Path to the mail server                                                    | string |
| `EMAIL_HOST_USER`             | Mail server login                                                          | string |
| `EMAIL_HOST_PASSWORD`         | Mail server password                                                       | string |
| `EMAIL_PORT`                  | Mail server port                                                           | int    |
| `EMAIL_FROM`                  | Mailing list email                                                         | string |
| `EMAIL_WORKERS`               | Number of parallel mail notification handlers of the application           | int    |
| `WEB_URL`                     | External address of the application                                        | string |
| `JITSI_DISABLED`              | Disabling jitsi                                                            | bool   |
| `JITSI_URL`                   | Address of the jitsi conferences                                           | string |
| `JITSI_JWT_SECRET`            | Secret key for jitsi auth JWT token                                        | string |
| `JITSI_APP_ID`                | Jitsi app ID for JWT iss field                                             | string |
| `FRONT_PATH`                  | The path to the compiled front (if specified, the back will return static) | string |
| `NOTIFICATIONS_PERIOD`        | Time period of a batch of email notifications                              | int    |
| `TELEGRAM_BOT_TOKEN`          | Telegram bot token                                                         | string |
| `TELEGRAM_COMMANDS_DISABLED`  | Disabling telegram bot commands                                            | bool   |
| `SESSIONS_DB_PATH`            | Path to the session database file                                          | string |
| `SIGN_UP_ENABLE`              | Enabling registration in the                                               | bool   |
| `DEMO`                        | Demo mode                                                                  | bool   |
| `SWAGGER_ENABLED`             | Enabling the Swagger API documentation at /api/swagger                     | bool   |
| `NY_ENABLE`                   | Enabling the New Year theme                                                | bool   |
| `CAPTCHA_DISABLED`            | Disabling captcha                                                          | bool   |

### nginx SSL example
```
server {
    listen 80;
    server_name aiplan.domain;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl;
    server_name aiplan.domain;

    ssl_certificate     /etc/ssl/certs/fullchain.pem;
    ssl_certificate_key /etc/ssl/private/privkey.pem;


    client_max_body_size 50M;

    location / {
        # we proxy directly to the container if it was launched in the same docker-compose with the same network, otherwise we change to the container IP/machine IP/localhost
        proxy_pass http://server:8080;
        proxy_http_version 1.1;

        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # WebSocket
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```
