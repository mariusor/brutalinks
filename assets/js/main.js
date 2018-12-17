$(document).ready(function() {
    // let _User = JSON.parse($("#currentUser").html());
    //console.debug(_User);

    let isInverted = Cookies.get("inverted") || false;
    if (isInverted && $("body").filter(".inverted").length == 0) {
        $("body").addClass("inverted");
        Cookies.set("inverted", true);
    }
    if (!isInverted && $("body").filter(".inverted").length == 1) {
        $("body").removeClass("inverted");
        Cookies.remove("inverted");
    }

    $("#top-invert").click(function(e) {
        let isInverted = Cookies.get("inverted") || false;
        $("html").toggleClass("inverted");
        if (isInverted) {
            Cookies.remove("inverted");
        } else {
            Cookies.set("inverted", true);
        }
        e.preventDefault();
        e.stopPropagation();
    });

    $("a.rm").click(function(e) {
        e.stopPropagation();
        e.preventDefault();

        $(".rm-confirm").remove();

        let el = $(e.delegateTarget);
        let hash = el.data("hash");

        let yesId = "yes-" + hash
        let noId = "no-" + hash

        el.after('<span class="rm-confirm">Confirm: <a href="#'+yesId+'" id="'+yesId+'">yes</a> <a href="#'+noId+'" id="'+noId+'">no</a></span>');
        $("a#" + yesId).click(function () {
            window.location = el.attr("href");
        });
        $("a#" + noId).click(function () {
            $(".rm-confirm").remove();
        });
    });

});
