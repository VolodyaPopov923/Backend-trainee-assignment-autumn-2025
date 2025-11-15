import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate } from 'k6/metrics';
import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';

const BASE = __ENV.BASE_URL || 'http://localhost:8080';
const ADMIN = 'admin';
const USER  = 'user';

export const serverErrors = new Rate('server_errors');

export const options = {
    discardResponseBodies: false,
    thresholds: {
        'http_req_duration{expected_response:true}': ['p(95)<300'],
        'server_errors': ['rate<0.001'],
    },
    scenarios: {
        spike: {
            executor: 'ramping-arrival-rate',
            startRate: 2,
            preAllocatedVUs: 20,
            timeUnit: '1s',
            stages: [
                { target: 5, duration: '30s' },
                { target: 8, duration: '1m'  },
                { target: 3, duration: '30s' },
            ],
        }
    }
};

function auth(h, token) {
    h['Authorization'] = `Bearer ${token}`;
    h['Content-Type']  = 'application/json';
    return h;
}

function track(res) {
    serverErrors.add(res.status >= 500);
}

function mkTeamOnce() {
    const body = JSON.stringify({
        team_name: 'backend',
        members: [
            { user_id: 'u1', username: 'Alice', is_active: true },
            { user_id: 'u2', username: 'Bob',   is_active: true },
            { user_id: 'u3', username: 'Carol', is_active: true },
            { user_id: 'u4', username: 'Dave',  is_active: true },
            { user_id: 'u5', username: 'Eve',   is_active: true }
        ]
    });
    const res = http.post(`${BASE}/team/add`, body, { headers: auth({}, ADMIN) });
    track(res);
    check(res, { 'team/add ok': r => r.status === 201 || r.status === 400 });
}

export function setup() {
    mkTeamOnce();
}

export default function () {
    const prId = `pr-${uuidv4()}`;
    const createBody = JSON.stringify({
        pull_request_id: prId,
        pull_request_name: 'Feature X',
        author_id: 'u1'
    });
    const resC = http.post(`${BASE}/pullRequest/create`, createBody, { headers: auth({}, ADMIN) });
    track(resC);
    check(resC, { 'create 201': r => r.status === 201 });
    if (resC.status !== 201) return;

    let assigned = [];
    try {
        const json = resC.json();
        assigned = (json && json.pr && json.pr.assigned_reviewers) || [];
    } catch (_) {}

    if (assigned.length > 0) {
        const old = assigned[0];
        const reBody = JSON.stringify({ pull_request_id: prId, old_user_id: old });
        const resR = http.post(`${BASE}/pullRequest/reassign`, reBody, { headers: auth({}, ADMIN) });
        track(resR);
        check(resR, { 'reassign 200/409': r => r.status === 200 || r.status === 409 });
    }

    const mBody = JSON.stringify({ pull_request_id: prId });
    const resM = http.post(`${BASE}/pullRequest/merge`, mBody, { headers: auth({}, ADMIN) });
    track(resM);
    check(resM, { 'merge 200': r => r.status === 200 });

    const resU = http.get(`${BASE}/users/getReview?user_id=u2`, { headers: auth({}, USER) });
    track(resU);
    check(resU, { 'getReview 200': r => r.status === 200 });

    sleep(0.1);
}
