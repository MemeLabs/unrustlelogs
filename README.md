# UnRustleLogs

## Setup

```
git clone https://github.com/tensei/unrustlelogs.git
cd ./unrustlelogs

mv example.config.toml config.toml

# edit the config.toml, make sure you provide everything needed
# otherwise some stuff might break
vim config.toml

mv ./package/etc/nginx/sites-available/unrustlelogs.conf /etc/nginx/sites-available/unrustlelogs

ln -s /etc/nginx/sites-available/unrustlelogs /etc/nginx/sites-enabled

docker-compose up -d --build
```
