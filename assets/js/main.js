OnReady( function() {
    let isInverted = function () { return getCookie("inverted") == "true" || false; };
    let haveModals = function() { return (typeof  document.createElement('dialog').showModal === "function"); };

    let root = $("html")[0];
    if (isInverted()) {
        root.classList.add("inverted");
    } else {
        root.classList.remove("inverted");
    }
    addEvent($("#invert")[0], "click", function(e) {
        if (isInverted()) {
            root.classList.remove("inverted");
            deleteCookie("inverted");
        } else {
            root.classList.add("inverted");
            setCookie("inverted", true);
        }
        e.preventDefault();
        e.stopPropagation();
    });
    $(".score a").forEach(function(lnk) {
        if(lnk.getAttribute("href") != "#") { return; }
        addEvent(lnk, "click", function(e){
            e.stopPropagation();
            e.preventDefault();
        });
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
            conf.innerHTML = ': <a href="#'+yesId+'" id="'+yesId+'">yes</a> / <a href="#'+noId+'" id="'+noId+'">no</a>';
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
    $("button[type='reset']").forEach(function (btn) {
        addEvent(btn, "click", function(e) {
            let backHref = btn.getAttribute("data-back");
            if (backHref == undefined) { return; }
            if (window.location.href.endsWith(backHref)) { return; }
            if (backHref.length > 0) {
                window.location = backHref;
            } else {
                history.go(-1);
            }
        });
    });
    if (haveModals()) {
        $("button.close").forEach(function (close) {
            addEvent(close, "click", function(e) {
                e.stopPropagation();
                e.preventDefault();
                let el = e.target.closest("dialog");
                el.close();
            });
        });
    }
});
