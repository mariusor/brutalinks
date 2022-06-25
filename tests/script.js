import { sleep, group, check } from 'k6';
import { parseHTML } from 'k6/html';
import http from 'k6/http';

export const options = {
    insecureSkipTLSVerify: true,
    thresholds: {},
    scenarios: {
        RegularBrowsing: {
            executor: 'ramping-vus',
            gracefulStop: '30s',
            stages: [
                { target: 20, duration: '1m' },
            ],
            gracefulRampDown: '30s',
            exec: 'regular_browsing',
        },
    },
}

const BASE_URL = "https://brutalinks.git";
/*
const PASSWORD = 'Sup3rS3cretS3cr3tP4ssW0rd!';
export function setup() {
    for (let i = 1; i <= 10; i++) {
        let response = http.get(`${BASE_URL}/register/`);
        check(response, {
                'user was created': (r) => r.status === 200
            }
        );

        let username = `user_${i}`;
        response = response.submitForm({
            formSelector: 'form',
            fields: {
                'handle': username,
                'pw': PASSWORD,
                'pw-confirm': PASSWORD,
            },
        });
        check(response, {
            'is status 200': r => r.status === 200,
        })

        response = http.get(`${BASE_URL}/~${username}`);
        check(response, {
            'is status 200': r => r.status === 200,
        });
    }
}
*/

const mapping = {
    'Homepage': {
        'path': '/',
        'title': 'Newest items',
    },
    'Local tab': {
        'path': '/self',
        'title': 'Local instance items',
    },
    'Federated tab': {
        'path': '/federated',
        'title': 'Federated items',
    },
    'Login': {
        'path': '/login',
        'title': 'Local authentication',
    },
    'Register': {
        'path': '/register',
        'title': 'Register new account',
    },
    'Users listing': {
        'path': '/~',
        'title': 'Account listing',
    },
    /*
    'Followed tab': {
        'path': '/followed',
        'title': 'Followed items',
    },
    'User page %USERNAME%': {
        'path': '/~%USERNAME%',
        'title': '%USER%&#39;s submissions',
    },
    'New submission page': {
        'path': '/submit',
        'title': 'Add new submission',
    },
     */
};

function checkAssets(doc) {
    let styles = doc.find('link[rel=stylesheet]');
    styles.map(
        function (idx, el) {
            group(`stylesheets[${styles.size()}]`, function () {
                let styleURL = el.attr('href');
                group(`${styleURL}`, function() {
                    let response = http.get(`${BASE_URL}${styleURL}`)
                    check(response, {
                        'is status 200': (r) => r.status === 200,
                        'is css': (r) => r.headers["Content-Type"] === 'text/css; charset=utf-8',
                    });
                });
            });
        }
    );

    let scripts  = doc.find('script[src]');
    scripts.map((idx, el) => {
        group(`scripts[${scripts.size()}]`, function () {
            let scriptURL = el.attr('src');
            group(`${scriptURL}`, function() {
                let response = http.get(`${BASE_URL}${scriptURL}`)
                check(response, {
                    'is status 200': (r) => r.status === 200,
                    'is js': (r) => r.headers["Content-Type"] === 'text/javascript; charset=utf-8',
                });
            });
        });
    });
}

export function regular_browsing() {
    group('/icons.svg', function () {
        let response = http.get(`${BASE_URL}/icons.svg`)
        check(response, {
            'is status 200': (r) => r.status === 200,
            'is svg': (r) => r.headers["Content-Type"] === 'image/svg+xml',
        });
    });

    for (let m in mapping) {
        group(m, function () {
            let response = http.get(`${BASE_URL}${mapping[m].path}`);
            check(response, {
                'is status 200': r => r.status === 200,
            });

            const doc = parseHTML(response.body);
            // Check title matches
            check(doc.find('head title').text(), {
                'has correct title': (s) => s === mapping[m].title,
            });
            checkAssets(doc);
        });

    }
};
