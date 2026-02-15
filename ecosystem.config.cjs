module.exports = {
    apps: [
        {
            name: 'angple-backend',
            script: './angple-backend',
            cwd: '/home/damoang/angple-backend',
            instances: 1,
            autorestart: true,
            watch: false,
            max_memory_restart: '500M',
            env: {
                APP_ENV: 'prod'
            }
        }
    ]
};
