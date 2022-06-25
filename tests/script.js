import { sleep, group, check } from 'k6'
import http from 'k6/http'

export const options = {
    insecureSkipTLSVerify: true,
    ext: {
        loadimpact: {
            projectID: 3590803,
            name: "Brutalinks-Local"
        }
    },
    thresholds: {},
    scenarios: {
        RegularBrowsing: {
            executor: 'ramping-vus',
            gracefulStop: '30s',
            stages: [
                { target: 20, duration: '1m' },
                { target: 20, duration: '3m30s' },
                { target: 0, duration: '1m' },
            ],
            gracefulRampDown: '30s',
            exec: 'regular_browsing',
        },
    },
}

const BASE_URL = "https://brutalinks.git";
const PASSWORD = 'Sup3rS3cretS3cr3tP4ssW0rd!';

export function setup() {
    for (let i = 1; i <= 10; i++) {
        let response = http.get(`${BASE_URL}/register/`);
        check(response, { 'created user': (r) => r.status === 200 });

        response = response.submitForm({
            formSelector: 'form',
            fields: {
                'handle': `user_${i}`,
                'pw': PASSWORD,
                'pw-confirm': PASSWORD,
            },
        });
        check(response, {
            'is status 200': r => r.status === 200,
        })
    }
}

const get_headers = {
    accept: 'text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8',
    'accept-language': 'en-US,en;q=0.5',
    'accept-encoding': 'gzip, deflate, br',
}

export function regular_browsing() {
    let response;

    group('Homepage', function () {
        response = http.get(`${BASE_URL}/`, {
            headers: get_headers,
        })
        check(response, {
            'is status 200': r => r.status === 200,
        })

        response = http.get(`${BASE_URL}/css/listing.css`, {
            headers: get_headers,
        })
        check(response, {
            'is status 200': r => r.status === 200,
        })
        response = http.get(`${BASE_URL}/css/l.css`, {
            headers: get_headers,
        })
        check(response, {
            'is status 200': r => r.status === 200,
        })
        response = http.get(`${BASE_URL}/icons.svg`, {
            headers: get_headers,
        })
        check(response, {
            'is status 200': r => r.status === 200,
        })
        response = http.get(`${BASE_URL}/js/main.js`, {
            headers: get_headers,
        })
        check(response, {
            'is status 200': r => r.status === 200,
        })
        response = http.get(`${BASE_URL}/css/m.css`, {
            headers: get_headers,
        })
        check(response, {
            'is status 200': r => r.status === 200,
        })
        response = http.get(`${BASE_URL}/css/s.css`, {
            headers: get_headers,
        })
        check(response, {
            'is status 200': r => r.status === 200,
        })
        sleep(3.7)
    })

    group('Homepage', function () {
        response = http.get(`${BASE_URL}/js/main.js`, {
            headers: get_headers,
        })
        check(response, {
            'is status 200': r => r.status === 200,
        })
        response = http.get(`${BASE_URL}/icons.svg`, {
            headers: get_headers,
        })
        check(response, {
            'is status 200': r => r.status === 200,
        })
        sleep(3.7)
    })

    group(
        'Navigate next page #1',
        function () {
            response = http.get(`${BASE_URL}/?after=2f5ed758-f9db-4af5-a059-2d8f3e43ab9f`, {
                headers: get_headers,
            })
            check(response, {
                'is status 200': r => r.status === 200,
            })
            sleep(1)
            response = http.get(`${BASE_URL}/js/main.js`, {
                headers: get_headers,
            })
            check(response, {
                'is status 200': r => r.status === 200,
            })
            response = http.get(`${BASE_URL}/icons.svg`, {
                headers: get_headers,
            })
            check(response, {
                'is status 200': r => r.status === 200,
            })
            sleep(3.2)
        }
    )

    group(
        'Navigate next page #2',
        function () {
            response = http.get(`${BASE_URL}/?after=af0d8cc3-ea0e-4e08-990d-43e93b3707c6`, {
                headers: get_headers,
            })
            check(response, {
                'is status 200': r => r.status === 200,
            })
            sleep(1.3)
            response = http.get(`${BASE_URL}/js/main.js`, {
                headers: get_headers,
            })
            check(response, {
                'is status 200': r => r.status === 200,
            })
            response = http.get(`${BASE_URL}/icons.svg`, {
                headers: get_headers,
            })
            check(response, {
                'is status 200': r => r.status === 200,
            })
            sleep(3.9)
        }
    )

    group(
        'Navigate next page #3',
        function () {
            response = http.get(`${BASE_URL}/?after=a049dd28-7af4-41ac-a6ff-7f9ba62a2bad`, {
                headers: get_headers,
            })
            check(response, {
                'is status 200': r => r.status === 200,
            })
            sleep(1.3)
            response = http.get(`${BASE_URL}/js/main.js`, {
                headers: get_headers,
            })
            check(response, {
                'is status 200': r => r.status === 200,
            })
            response = http.get(`${BASE_URL}/icons.svg`, {
                headers: get_headers,
            })
            check(response, {
                'is status 200': r => r.status === 200,
            })
            sleep(3.6)
        }
    )

    group(
        'Navigate next page #4',
        function () {
            response = http.get(`${BASE_URL}/?after=fd6f28ad-7231-4a3e-83d7-9608c0eea3f0`, {
                headers: get_headers,
            })
            check(response, {
                'is status 200': r => r.status === 200,
            })
            sleep(1.3)
            response = http.get(`${BASE_URL}/js/main.js`, {
                headers: get_headers,
            })
            check(response, {
                'is status 200': r => r.status === 200,
            })
            response = http.get(`${BASE_URL}/icons.svg`, {
                headers: get_headers,
            })
            check(response, {
                'is status 200': r => r.status === 200,
            })
            sleep(5.1)
        }
    )

    group(
        'Navigate next page #5',
        function () {
            response = http.get(`${BASE_URL}/?after=13353393-2676-4516-9805-203b230c6a70`, {
                headers: get_headers,
            })
            check(response, {
                'is status 200': r => r.status === 200,
            })
            sleep(0.7)
            response = http.get(`${BASE_URL}/js/main.js`, {
                headers: get_headers,
            })
            check(response, {
                'is status 200': r => r.status === 200,
            })
            response = http.get(`${BASE_URL}/icons.svg`, {
                headers: get_headers,
            })
            check(response, {
                'is status 200': r => r.status === 200,
            })
            sleep(4.4)
        }
    )

    group(
        'Navigate next page #6',
        function () {
            response = http.get(`${BASE_URL}/~marius/2e6cf5a8-ffa8-4fca-b4cd-8e0c9a584a75`, {
                headers: get_headers,
            })
            check(response, {
                'is status 200': r => r.status === 200,
            })
            sleep(0.8)
            response = http.get(`${BASE_URL}/css/content.css`, {
                headers: get_headers,
            })
            check(response, {
                'is status 200': r => r.status === 200,
            })
            response = http.get(`${BASE_URL}/js/main.js`, {
                headers: get_headers,
            })
            check(response, {
                'is status 200': r => r.status === 200,
            })
            response = http.get(`${BASE_URL}/icons.svg`, {
                headers: get_headers,
            })
            check(response, {
                'is status 200': r => r.status === 200,
            })
            sleep(8.3)
        }
    )

    group('Moderation', function () {
        response = http.get(`${BASE_URL}/moderation`, {
            headers: get_headers,
        })
        check(response, {
            'is status 200': r => r.status === 200,
        })
        sleep(0.5)
        response = http.get(`${BASE_URL}/css/moderation.css`, {
            headers: get_headers,
        })
        check(response, {
            'is status 200': r => r.status === 200,
        })
        response = http.get(`${BASE_URL}/js/main.js`, {
            headers: get_headers,
        })
        check(response, {
            'is status 200': r => r.status === 200,
        })
        response = http.get(`${BASE_URL}/icons.svg`, {
            headers: get_headers,
        })
        check(response, {
            'is status 200': r => r.status === 200,
        })
        sleep(5.5)
    })

    group(
        'Moderation next page #1',
        function () {
            response = http.get(
                `${BASE_URL}/moderation?after=f1609fe1-f46f-4f70-802f-5dd75034471f`,
                {
                    headers: get_headers,
                }
            )
            check(response, {
                'is status 200': r => r.status === 200,
            })
            sleep(0.7)
            response = http.get(`${BASE_URL}/js/main.js`, {
                headers: get_headers,
            })
            response = http.get(`${BASE_URL}/icons.svg`, {
                headers: get_headers,
            })
            check(response, {
                'is status 200': r => r.status === 200,
            })
            sleep(3.3)
        }
    )

    group('Moderation just comments', function () {
        response = http.get(`${BASE_URL}/moderation?t=c`, {
            headers: get_headers,
        })
        check(response, {
            'is status 200': r => r.status === 200,
        })
        sleep(0.5)
        response = http.get(`${BASE_URL}/js/main.js`, {
            headers: get_headers,
        })
        check(response, {
            'is status 200': r => r.status === 200,
        })
        response = http.get(`${BASE_URL}/icons.svg`, {
            headers: get_headers,
        })
        check(response, {
            'is status 200': r => r.status === 200,
        })
        sleep(3.4)
    })

    group('Moderation just submissions', function () {
        response = http.get(`${BASE_URL}/moderation?t=s`, {
            headers: get_headers,
        })
        check(response, {
            'is status 200': r => r.status === 200,
        })
        sleep(0.7)
        response = http.get(`${BASE_URL}/js/main.js`, {
            headers: get_headers,
        })
        check(response, {
            'is status 200': r => r.status === 200,
        })
        response = http.get(`${BASE_URL}/icons.svg`, {
            headers: get_headers,
        })
        check(response, {
            'is status 200': r => r.status === 200,
        })
        sleep(5.7)
    })

    group('Followed page', function () {
        response = http.get(`${BASE_URL}/followed`, {
            headers: get_headers,
        })
        check(response, {
            'is status 200': r => r.status === 200,
        })
        sleep(2.7)
        response = http.get(`${BASE_URL}/js/main.js`, {
            headers: get_headers,
        })
        check(response, {
            'is status 200': r => r.status === 200,
        })
        response = http.get(`${BASE_URL}/icons.svg`, {
            headers: get_headers,
        })
        check(response, {
            'is status 200': r => r.status === 200,
        })
        sleep(3.9)
    })

    group('Self page', function () {
        response = http.get(`${BASE_URL}/self`, {
            headers: get_headers,
        })
        check(response, {
            'is status 200': r => r.status === 200,
        })
        sleep(0.5)
        response = http.get(`${BASE_URL}/js/main.js`, {
            headers: get_headers,
        })
        check(response, {
            'is status 200': r => r.status === 200,
        })
        response = http.get(`${BASE_URL}/icons.svg`, {
            headers: get_headers,
        })
        check(response, {
            'is status 200': r => r.status === 200,
        })
        sleep(5.4)
    })

    group('Logout', function () {
        response = http.get(`${BASE_URL}/logout`, {
            headers: get_headers,
        })
        check(response, {
            'is status 200': r => r.status === 200,
        })
        response = http.get(`${BASE_URL}/js/main.js`, {
            headers: get_headers,
        })
        check(response, {
            'is status 200': r => r.status === 200,
        })
        response = http.get(`${BASE_URL}/icons.svg`, {
            headers: get_headers,
        })
        check(response, {
            'is status 200': r => r.status === 200,
        })
    })
}
