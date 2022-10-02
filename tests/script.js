import {check, fail, group, sleep} from 'k6';
import {parseHTML} from 'k6/html';
import http from 'k6/http';

export const options = {
    thresholds: {
        http_req_failed: ['rate<0.04'], // http errors should be less than 4%
        'http_req_duration{type:content}': ['p(95)<200'], // threshold on API requests only
        'http_req_duration{type:static}': ['p(95)<100'], // threshold on static content only

    },
    scenarios: {
        RegularBrowsing: {
            executor: 'per-vu-iterations',
            vus: 2,
            iterations: 10,
            maxDuration: '1s',
            exec: 'regularBrowsing',
        },
    },
}

const BASE_URL = __ENV.TEST_HOST;

export function setup() {
    for (let i in users) {
        let u = users[i];
        let response = http.get(`${BASE_URL}/register/`);
        check(response, {
                'user was created': (r) => r.status === 200
            }
        );

        response = response.submitForm({
            formSelector: 'form',
            fields: {
                'handle': u.handle,
                'pw': u.pw,
                'pw-confirm': u.pw,
            },
        });
        check(response, {
            'is status 200': r => r.status === 200,
        })

        response = http.get(`${BASE_URL}/~${u.handle}`);
        check(response, {
            'is status 200': r => r.status === 200,
        });
    }
}

const PASSWORD = 'Sup3rS3cretS3cr3tP4ssW0rd!';
const users = [
    {
        handle: 'user_test_0',
        pw: PASSWORD,
    }
];


const pages = {
    'About': {
        path: '/about',
        title: 'About',
    },
    'Homepage': {
        path: '/',
        title: 'Newest items',
    },
    'Local tab': {
        path: '/self',
        title: 'Local instance items',
    },
    'Federated tab': {
        path: '/federated',
        title: 'Federated items',
    },
    'Tags': {
        path: '/t/tags',
        title: 'Items tagged as #tags',
    },
    'Discussions': {
        path: '/d',
        title: 'Discussion items',
    },
    'Login': {
        path: '/login',
        title: 'Local authentication',
    },
    'Register': {
        path: '/register',
        title: 'Register new account',
    },
    'Users listing': {
        path: '/~',
        title: 'Account listing',
    },
    'Moderation': {
        path: '/moderation',
        title: 'Moderation log',
        user: users[0],
    },
    'Followed tab': {
        path: '/followed',
        title: 'Followed items',
        user: users[0],
    },
    'User page': {
        path: `/~${users[0].handle}`,
        title: `${users[0].handle}'s submissions`,
        user: users[0],
    },
    'New submission page': {
        path: '/submit',
        title: 'Add new submission',
        user: users[0],
    },
};

function authenticate(u) {
    let response = http.get(`${BASE_URL}/login`);
    check(response, {
            'login page': (r) => r.status === 200
        }
    );

    response = response.submitForm({
        formSelector: 'form',
        fields: u,
    });
    check(response, {
        'is status 200': r => r.status === 200,
        'has auth cookie': r => function (r) {
            console.log(r);
        },
    })
};

function checkTitle(test, doc) {
    // Check title matches
    let tit = doc.find('head title').text();
    if (!check(tit, {
        'has correct title': (s) => s === test.title,
    })) {
        fail(`Title "${tit}", expected "${test.title}"`)
    }
}

function checkAssets(test, doc) {
    let styles = doc.find('link[rel=stylesheet]');
    styles.map(
        function (idx, el) {
            group(`stylesheets[${styles.size()}]`, function () {
                let styleURL = el.attr('href');
                group(`${styleURL}`, function () {
                    let response = http.get(`${BASE_URL}${styleURL}`, {tags: {type: 'static'}})
                    check(response, {
                        'OK': (r) => r.status === 200 && r.headers["Content-Type"] === 'text/css; charset=utf-8',
                    });
                });
            });
        }
    );

    let scripts = doc.find('script[src]');
    scripts.map((idx, el) => {
        group(`scripts[${scripts.size()}]`, function () {
            let scriptURL = el.attr('src');
            group(`${scriptURL}`, function () {
                let response = http.get(`${BASE_URL}${scriptURL}`, {tags: {type: 'static'}})
                check(response, {
                    'OK': (r) => r.status === 200 && r.headers["Content-Type"] === 'text/javascript; charset=utf-8',
                });
            });
        });
    });
}

function pageAssertions(test) {
    return function () {
        let response = http.get(`${BASE_URL}${test.path}`, {tags: {type: 'content'}});
        check(response, {
            'is status 200': r => r.status === 200,
        });

        const doc = parseHTML(response.body);
        checkTitle(test, doc);
        checkAssets(test, doc);
    }
};

export function regularBrowsing() {
    group('/icons.svg', function () {
        let response = http.get(`${BASE_URL}/icons.svg`, {tags: {type: 'static'}})
        check(response, {
            'is status 200': (r) => r.status === 200,
            'is svg': (r) => r.headers["Content-Type"] === 'image/svg+xml',
        });
    });

    for (let m in pages) {
        let test = pages[m];
        if (test.hasOwnProperty("user")) {
            let u = test.user;
            authenticate(u);
            m = `${m}: ${u.handle}`
        }
        group(m, pageAssertions(test));
        sleep(0.1);
    }
};

