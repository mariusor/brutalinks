import {check, fail, group, sleep} from 'k6';
import {parseHTML} from 'k6/html';
import http from 'k6/http';

export const options = {
    thresholds: {
        'http_req_failed{type:content}': ['rate<0.03'], // http errors should be less than 4%
        'http_req_failed{type:static}': ['rate<0.01'], // http errors should be less than 1%
        'http_req_duration{type:content}': ['p(50)<150'], // threshold on API requests only under 150ms
        'http_req_duration{type:static}': ['p(50)<5'], // threshold on static content only under 5ms
    },
    scenarios: {
        regular_browsing: {
            executor: 'constant-vus',
            vus: 4,
            duration: '10s',
            exec: 'regularBrowsing',
            gracefulStop: 0,
        },
    },
}

const BASE_URL = __ENV.TEST_HOST;

export function setup() {
    for (let i in users) {
        let u = users[i];
        if (check(http.get(`${BASE_URL}/~${u.handle}`), {
            'user exists': (r) => r.status === 200,
        })) {
            return;
        }

        let response = http.get(`${BASE_URL}/register`).submitForm({
            formSelector: 'form',
            fields: {
                'handle': u.handle,
                'pw': u.pw,
                'pw-confirm': u.pw,
            },
        });
        check(response, {
            'user created': r => r.status === 200,
        })

        response = http.get(`${BASE_URL}/~${u.handle}`);
        check(response, {
            'user exists': r => r.status === 200,
        });
    }
}

const PASSWORD = 'Sup3rS3cretS3cr3tP4ssW0rd!';
const users = [
    {
        handle: 'admin',
        pw: PASSWORD,
    },
    {
        handle: 'user_test_0',
        pw: PASSWORD,
    }
];


const pages = {
    '/css/moderate.css': {
        path: '/css/moderate.css',
        tags: {type: 'static'},
        checks: CSSChecks(),
    },
    '/css/content.css': {
        path: '/css/content.css',
        tags: {type: 'static'},
        checks: CSSChecks(),
    },
    '/css/accounts.css': {
        path: '/css/accounts.css',
        tags: {type: 'static'},
        checks: CSSChecks(),
    },
    '/css/listing.css': {
        path: '/css/listing.css',
        tags: {type: 'static'},
        checks: CSSChecks(),
    },
    '/css/moderation.css': {
        path: '/css/moderation.css',
        tags: {type: 'static'},
        checks: CSSChecks(),
    },
    '/css/user.css': {
        path: '/css/user.css',
        tags: {type: 'static'},
        checks: CSSChecks(),
    },
    '/css/user-message.css': {
        path: '/css/user-message.css',
        tags: {type: 'static'},
        checks: CSSChecks(),
    },
    '/css/new.css': {
        path: '/css/new.css',
        tags: {type: 'static'},
        checks: CSSChecks(),
    },
    '/css/404.css': {
        path: '/css/404.css',
        tags: {type: 'static'},
        checks: CSSChecks(),
    },
    '/css/about.css': {
        path: '/css/about.css',
        tags: {type: 'static'},
        checks: CSSChecks(),
    },
    '/css/error.css': {
        path: '/css/error.css',
        tags: {type: 'static'},
        checks: CSSChecks(),
    },
    '/css/login.css': {
        path: '/css/login.css',
        tags: {type: 'static'},
        checks: CSSChecks(),
    },
    '/css/register.css': {
        path: '/css/register.css',
        tags: {type: 'static'},
        checks: CSSChecks(),
    },
    '/css/inline.css': {
        path: '/css/inline.css',
        tags: {type: 'static'},
        checks: CSSChecks(),
    },
    '/css/simple.css': {
        path: '/css/simple.css',
        tags: {type: 'static'},
        checks: CSSChecks(),
    },
    '/css/l.css': {
        path: '/css/l.css',
        tags: {type: 'static'},
        checks: CSSChecks(),
    },
    '/css/m.css': {
        path: '/css/m.css',
        tags: {type: 'static'},
        checks: CSSChecks(),
    },
    '/css/s.css': {
        path: '/css/s.css',
        tags: {type: 'static'},
        checks: CSSChecks(),
    },
    '/js/main.js': {
        path: '/js/main.js',
        tags: {type: 'static'},
        checks: {
            'is status 200': isOK,
            'is JavaScript': isJavaScript,
        }
    },
    '/robots.txt': {
        path: '/robots.txt',
        tags: {type: 'static'},
        checks: {
            'is status 200': isOK,
            'is text': isPlainText,
        }
    },
    '/favicon.ico': {
        path: '/favicon.ico',
        tags: {type: 'static'},
        checks: {
            'is status 200': isOK,
            'is ns': (r) => contentType(r) === ' image/vnd.microsoft.icon',
        }
    },
    '/icons.svg': {
        path: '/icons.svg',
        tags: {type: 'static'},
        checks: {
            'is status 200': isOK,
            'is svg': (r) => contentType(r) === 'image/svg+xml',
        }
    },
    'About': {
        path: '/about',
        tags: {type: 'content'},
        checks: HTMLChecks('About'),
    },
    'Homepage': {
        path: '/',
        tags: {type: 'content'},
        checks: HTMLChecks('Newest items')
    },
    'Local tab': {
        path: '/self',
        tags: {type: 'content'},
        checks: HTMLChecks('Local instance items'),
    },
    'Federated tab': {
        path: '/federated',
        tags: {type: 'content'},
        checks: HTMLChecks('Federated items'),
    },
    'Tags': {
        path: '/t/tags',
        tags: {type: 'content'},
        checks: HTMLChecks('Items tagged as #tags'),
    },
    'Discussions': {
        path: '/d',
        tags: {type: 'content'},
        checks: HTMLChecks('Discussion items'),
    },
    'Login': {
        path: '/login',
        tags: {type: 'content'},
        checks: HTMLChecks('Local authentication'),
    },
    'Register': {
        path: '/register',
        tags: {type: 'content'},
        checks: HTMLChecks('Register new account'),
    },
    'Users listing': {
        path: '/~',
        tags: {type: 'content'},
        checks: HTMLChecks('Account listing'),
    },
    'Moderation': {
        path: '/moderation',
        tags: {type: 'content'},
        checks: HTMLChecks('Moderation log'),
        user: users[0],
    },
    'Followed tab': {
        path: '/followed',
        tags: {type: 'content'},
        checks: HTMLChecks('Followed items'),
        user: users[0],
    },
    'User page': {
        path: `/~${users[0].handle}`,
        tags: {type: 'content'},
        checks: HTMLChecks(`${users[0].handle}'s submissions`),
        user: users[0],
    },
    'New submission page': {
        path: '/submit',
        tags: {type: 'content'},
        checks: HTMLChecks('Add new submission'),
        user: users[0],
    },
};

function HTMLChecks(title) {
    return {
        'status 200': isOK,
        'is HTML': isHTML,
        'has correct title': hasTitle(title),
    }
}

function CSSChecks() {
    return {
        'is status 200': isOK,
        'is CSS': isCSS,
    }
}

function isOK(r) {
    return r.status === 200
}

function isHTML(r) {
    return contentType(r) === 'text/html; charset=utf-8';
}

function isPlainText(r) {
    return contentType(r) === 'text/plain; charset=utf-8';
}

function isJavaScript(r) {
    return contentType(r) === 'text/javascript; charset=utf-8';
}

function hasTitle(s) {
    return (r) => htmlTitle(r) === s
}

function isCSS(r) {
    return contentType(r) == 'text/css; charset=utf-8';
}

const contentType = (r) => r.headers.hasOwnProperty('Content-Type') ? r.headers['Content-Type'].toLowerCase() : '';
const htmlTitle = (r) => parseHTML(r.body).find('head title').text();

function authenticate(u) {
    let response = http.get(`${BASE_URL}/login`);
    if (!check(response, { 'login page': isOK } )) {
        fail('invalid login response');
        return;
    }

    response = response.submitForm({
        formSelector: 'form',
        fields: u,
    });

    const cookiesForURL = http.cookieJar().cookiesForURL(response.url);
    check(response, {
        'is status 200': isOK,
        'has session cookie': () => cookiesForURL._s.length > 0,
    })
};

export function regularBrowsing() {
    for (let m in pages) {
        let test = pages[m];
        if (!test.hasOwnProperty("path")) {
            fail('invalid test element, missing "path" property')
        }
        if (!test.hasOwnProperty("checks")) {
            fail('invalid test element, missing "checks" property')
        }
        group(m, function () {
            if (test.hasOwnProperty("user")) {
                authenticate(test.user);
                m = `${m}: ${test.user.handle}`
            }

            check(
                http.get(`${BASE_URL}${test.path}`, {tags: test.tags}),
                test.checks
            );
        });
        sleep(0.1);
    }
};

