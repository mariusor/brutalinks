// add matches
this.Element && function(ElProt) {
    ElProt.matchesSelector = ElProt.matchesSelector ||
        ElProt.mozMatchesSelector ||
        ElProt.msMatchesSelector ||
        ElProt.oMatchesSelector ||
        ElProt.webkitMatchesSelector ||
        function (selector) {
            let node = this, nodes = (node.parentNode || node.document).querySelectorAll(selector), i = -1;
            while (nodes[++i] && nodes[i] != node);
            return !!nodes[i];
        };
    ElProt.matches = ElProt.matches || ElProt.matchesSelector;
}(Element.prototype);
// closest polyfill
this.Element && function(ElementPrototype) {
    ElementPrototype.closest = ElementPrototype.closest ||
    function(selector) {
        let el = this;
        while (el.matches && !el.matches(selector)) el = el.parentNode;
        return el.matches ? el : null;
    }
}(Element.prototype);
// helper for enabling IE 8 event bindings
let addEvent = function (el, type, handler) {
    if (el.attachEvent) el.attachEvent('on'+type, handler); else el.addEventListener(type, handler);
};
let removeEvent = function (el, type, handler) {
    if (el.detachEvent) el.detachEvent('on'+type, handler); else el.removeEventListener(type, handler);
};
// Cookie
let getCookie = function (name) {
    let v = document.cookie.match('(^|;) ?' + name + '=([^;]*)(;|$)');
    return v ? v[2] : null;
};
let setCookie = function (name, value, days=1000) {
    let d = new Date;
    d.setTime(d.getTime() + 24*60*60*1000*days);
    document.cookie = name + "=" + value + ";path=/;expires=" + d.toGMTString();
};
let deleteCookie = function (name) { setCookie(name, '', -1); };
// Document.Ready
let OnReady = function(fn) {
    // in case the document is already rendered
    if (document.readyState != 'loading') {
        fn.call();
    } else if (document.addEventListener) {
        // modern browsers
        document.addEventListener('DOMContentLoaded', fn);
    }
};
// pretend we're jquery
let $ = function (selector, context) {
    console.debug(selector, context);
    return (context || document).querySelectorAll(selector);
};
// Doing the work
OnReady( function() {
    // let _User = JSON.parse($("#currentUser").html());
    //console.debug(_User);

    let isInverted = getCookie("inverted") || false;
    if (isInverted) {
        $("body")[0].classList.add("inverted");
    } else {
        $("body")[0].classList.remove("inverted");
    }
    addEvent($("#top-invert")[0], "click", function(e) {
        let isInverted = getCookie("inverted") || false;
        if (isInverted) {
            $("html")[0].classList.remove("inverted");
            deleteCookie("inverted");
        } else {
            $("html")[0].classList.add("inverted");
            setCookie("inverted", true);
        }
        e.preventDefault();
        e.stopPropagation();
    });

    $("a.rm").forEach(function (del) {
        addEvent(del, "click", function(e) {
            e.stopPropagation();
            e.preventDefault();

            $(".rm-confirm").forEach(function (conf) {
                conf.parentNode && conf.parentNode.removeChild(conf);
            });

            let el = e.target.closest("a");
            let hash = el.getAttribute("data-hash");

            let yesId = "yes-" + hash;
            let noId = "no-" + hash;

            let conf = document.createElement('span');
            conf.classList.add("rm-confirm");
            conf.innerHTML = 'Remove? <a href="#'+yesId+'" id="'+yesId+'">yes</a> <a href="#'+noId+'" id="'+noId+'">no</a>';
            el.after(conf);
            addEvent($("a#" + yesId)[0], "click", function (e) {
                window.location = el.getAttribute("href");
                el.parentNode.removeChild(conf);
                e.stopPropagation();
                e.preventDefault();
            });
            addEvent($("a#" + noId)[0], "click", function (e) {
                el.parentNode.removeChild(conf);
                e.stopPropagation();
                e.preventDefault();
            });
        });
    });
});
