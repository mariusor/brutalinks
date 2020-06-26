this.Element && function (a) {
    a.matchesSelector = a.matchesSelector || a.mozMatchesSelector || a.msMatchesSelector || a.oMatchesSelector || a.webkitMatchesSelector || function (b) {
        let c = this, e = (c.parentNode || c.document).querySelectorAll(b), f = -1;
        for (; e[++f] && e[f] != c;) ;
        return !!e[f]
    }, a.matches = a.matches || a.matchesSelector
}(Element.prototype);
this.Element && function (a) {
    a.closest = a.closest || function (b) {
        let c = this;
        for (; c.matches && !c.matches(b);) c = c.parentNode;
        return c.matches ? c : null
    }
}(Element.prototype);
let addEvent = function (a, b, c) {
    a.attachEvent ? a.attachEvent('on' + b, c) : a.addEventListener(b, c)
};
let removeEvent = function (a, b, c) {
    a.detachEvent ? a.detachEvent('on' + b, c) : a.removeEventListener(b, c)
};
let getCookie = function (a) {
    let b = document.cookie.match('(^|;) ?' + a + '=([^;]*)(;|$)');
    return b ? b[2] : null
};
let setCookie = function (a, b, c = 1e3) {
    let e = new Date;
    e.setTime(e.getTime() + 86400000 * c), document.cookie = a + '=' + b + ';path=/;expires=' + e.toGMTString()
};
let deleteCookie = function (a) {
    setCookie(a, '', -1)
};
let OnReady = function (a) {
    'loading' == document.readyState ? document.addEventListener && document.addEventListener('DOMContentLoaded', a) : a.call()
};
let $ = function (a, b) {
    return console.debug(a, b), (b || document).querySelectorAll(a)
};
