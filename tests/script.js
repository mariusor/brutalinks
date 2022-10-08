import {check, fail, group, sleep} from 'k6';
import {parseHTML} from 'k6/html';
import http from 'k6/http';
import { Rate } from 'k6/metrics';

const errors = new Rate('error_rate');

export const options = {
    thresholds: {
        'http_req_failed{type:content}': ['rate<0.03'], // http errors should be less than 4%
        'http_req_failed{type:static}': ['rate<0.01'], // http errors should be less than 1%
        'http_req_duration{type:content}': ['p(50)<150'], // threshold on API requests only under 150ms
        'http_req_duration{type:static}': ['p(50)<5'], // threshold on static content only under 5ms
        'error_rate': [ { threshold: 'rate < 0.1', abortOnFail: true, delayAbortEval: '1m' } ],
        'error_rate{errorType:responseStatusError}': [ { threshold: 'rate < 0.1', }, ],
        'error_rate{errorType:contentTypeError}': [ { threshold: 'rate < 0.1', }, ],
        'error_rate{errorType:titleError}': [ { threshold: 'rate < 0.1', }, ],
        'error_rate{errorType:cookieMissingError}': [ { threshold: 'rate < 0.1', }, ],
        'error_rate{errorType:authorizationError}': [ { threshold: 'rate < 0.1', }, ],
        'error_rate{errorType:contentError}': [ { threshold: 'rate < 0.1', }, ],
    },
    scenarios: {
        regular_browsing: {
            executor: 'constant-vus',
            vus: 3,
            duration: '20s',
            exec: 'regularBrowsing',
            gracefulStop: '2s',
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


const staticResources = {
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
            'is favicon': isFavicon,
        }
    },
    '/icons.svg': {
        path: '/icons.svg',
        tags: {type: 'static'},
        checks: {
            'is status 200': isOK,
            'is svg': isSvg,
        }
    },
};

const pages = {
    'About': {
        path: '/about',
        tags: {type: 'content'},
        checks: Object.assign(
            HTMLChecks('About'),
            TabChecks('/self', '/federated'),
            checkAboutPage()
        ),
    },
    'Homepage': {
        path: '/',
        tags: {type: 'content'},
        checks: Object.assign(
            HTMLChecks('Newest items'),
            TabChecks('/self', '/federated'),
        ),
    },
    'Local tab': {
        path: '/self',
        tags: {type: 'content'},
        checks: Object.assign(
            HTMLChecks('Local instance items'),
            TabChecks('/self', '/federated'),
        ),
    },
    'Federated tab': {
        path: '/federated',
        tags: {type: 'content'},
        checks: Object.assign(
            HTMLChecks('Federated items'),
            TabChecks('/self', '/federated'),
        ),
    },
    'Tags': {
        path: '/t/tags',
        tags: {type: 'content'},
        checks: Object.assign(
            HTMLChecks('Items tagged as #tags'),
            TabChecks('/self', '/federated'),
        ),
    },
    'Discussions': {
        path: '/d',
        tags: {type: 'content'},
        checks: Object.assign(
            HTMLChecks('Discussion items'),
            TabChecks('/self', '/federated'),
        ),
    },
    'Login': {
        path: '/login',
        tags: {type: 'content'},
        checks: Object.assign(
            HTMLChecks('Local authentication'),
            TabChecks('/self', '/federated'),
        ),
    },
    'Register': {
        path: '/register',
        tags: {type: 'content'},
        checks: Object.assign(
            HTMLChecks('Register new account'),
            TabChecks('/self', '/federated'),
        ),
    },
    'Users listing': {
        path: '/~',
        tags: {type: 'content'},
        checks: Object.assign(
            HTMLChecks('Account listing'),
            TabChecks('/self', '/federated'),
        ),
    },
    'Moderation': {
        path: '/moderation',
        tags: {type: 'content'},
        checks: Object.assign(
            HTMLChecks('Moderation log'),
            TabChecks('/self', '/federated', '/followed', '/submit'),
        ),
        user: users[0],
    },
    'Followed tab': {
        path: '/followed',
        tags: {type: 'content'},
        checks: Object.assign(
            HTMLChecks('Followed items'),
            TabChecks('/self', '/federated', '/followed', '/submit'),
        ),
        user: users[0],
    },
    'Logged user\'s page': {
        path: `/~${users[0].handle}`,
        tags: {type: 'content'},
        checks: Object.assign(
            HTMLChecks(`${users[0].handle}'s submissions`),
            TabChecks('/self', '/federated', '/followed', '/submit'),
        ),
        user: users[0],
    },
    'New submission page': {
        path: '/submit',
        tags: {type: 'content'},
        checks: Object.assign(
            HTMLChecks('Add new submission'),
            TabChecks('/self', '/federated', '/followed', '/submit'),
        ),
        user: users[0],
    },
};

function checkAboutPage() {
    return {
        "main#about exists": (r) => parseHTML(r.body).find('main#about').size() === 1,
        "main#about content": (r) => parseHTML(r.body).find('main#about h1').text() === 'About',
    }
}

function hasLogo (r) {
    let logo = parseHTML(r.body).find('body header h1 svg title');
    let header = parseHTML(r.body).find('body header h1 a').children(':not(svg)');

    let status = header.text() === 'brutalinks(test)' && logo.text() === 'trash-o';
    errors.add(!status, {errorType: 'contentError'});
    return status;
}

function TabChecks() {
    const tabCount = arguments.length;
    const tabNames = arguments;

    let checks = {
        'has tabs': (r) => {
            let status = parseHTML(r.body).find('body header menu.tabs li').size() === tabCount;
            errors.add(!status, {errorType: 'contentError'});
            return status;
        }
    };

    for (let i = 0; i < arguments.length; i++) {
        const currentTab = tabNames[i]
        const key = `has tab: "${currentTab}"`;
        checks[key] = (r) => {
            let span = parseHTML(r.body).find('body header menu.tabs li a[href="' + currentTab+'"] span');
            let status = span.size() === 1 && span.text().replace('/', '') === currentTab.replace('/', '');
            errors.add(!status, {errorType: 'contentError'});
            return status;
        }
    }

    return checks;
}
function HTMLChecks(title) {
    return {
        'status 200': isOK,
        'is HTML': isHTML,
        'has correct title': hasTitle(title),
        'has logo': hasLogo,
    }
}

function CSSChecks() {
    return {
        'is status 200': isOK,
        'is CSS': isCSS,
    }
}

function isOK(r) {
    let status = r.status === 200
    errors.add(!status, {errorType: 'responseStatusError'});
    return status;
}

function isHTML(r) {
    let status = contentType(r) === 'text/html; charset=utf-8';
    errors.add(!status, {errorType: 'contentTypeError'});
    return status;
}

function isSvg(r) {
    let status = contentType(r) === 'image/svg+xml';
    errors.add(!status, {errorType: 'contentTypeError'});
    return status;
}

function isFavicon(r) {
    let status = contentType(r) === 'image/vnd.microsoft.icon';
    errors.add(!status, {errorType: 'contentTypeError'});
    return status;
}

function isPlainText(r) {
    let status = contentType(r) === 'text/plain; charset=utf-8';
    errors.add(!status, {errorType: 'contentTypeError'});
    return status;
}

function isJavaScript(r) {
    let status = contentType(r) === 'text/javascript; charset=utf-8';
    errors.add(!status, {errorType: 'contentTypeError'});
    return status;
}

function hasTitle(s) {
    return (r) => {
        let status = htmlTitle(r) === s;
        errors.add(!status, {errorType: 'titleError'});
        return status;
    }
}

function isCSS(r) {
    let status = contentType(r) == 'text/css; charset=utf-8';
    errors.add(!status, {errorType: 'contentTypeError'});
    return status;
}

const contentType = (r) => r.headers.hasOwnProperty('Content-Type') ? r.headers['Content-Type'].toLowerCase() : '';
const htmlTitle = (r) => parseHTML(r.body).find('head title').text();

function authenticate(u) {
    let response = http.get(`${BASE_URL}/login`);
    if (!check(response, { 'login page': isOK } )) {
        fail('invalid login response');
        errors.add(1, {errorType: 'authorizationError'});
        return;
    }

    response = response.submitForm({
        formSelector: 'form',
        fields: u,
    });

    const cookiesForURL = http.cookieJar().cookiesForURL(response.url);
    check(response, {
        'is status 200': isOK,
        'has session cookie': () => {
            let status = cookiesForURL._s.length > 0;
            errors.add(!status, {errorType: 'authorizationError'});
            return status;
        },
    })
};

function runSuite(pages, sleepTime = 0) {
    return () => {
        for (let m in pages) {
            let test = pages[m];
            if (!test.hasOwnProperty('path')) {
                fail('invalid test element, missing "path" property');
                return;
            }
            if (!test.hasOwnProperty('checks')) {
                fail('invalid test element, missing "checks" property');
                return;
            }
            group(m, function () {
                if (test.hasOwnProperty('user')) {
                    authenticate(test.user);
                    m = `${m}: ${test.user.handle}`;
                }

                check(
                    http.get(`${BASE_URL}${test.path}`, {tags: test.tags}),
                    test.checks
                );
                sleep(sleepTime);
            });
        }
    }
}

export function regularBrowsing() {
    group('StaticResources', runSuite(staticResources));
    group('Content', runSuite(pages), 0.1);
};
