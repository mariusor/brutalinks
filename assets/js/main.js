$( document ).ready(function() {

    var _User = JSON.parse($("#currentUser").html());
    console.debug(_User);

    var isInverted = Cookies.get("inverted") || false;

    if (isInverted && $(":root").filter(".inverted").length == 0) {
        $(":root").addClass("inverted");
        Cookies.set("inverted", true);
    }
    if (!isInverted && $(":root").filter(".inverted").length == 1) {
        $(":root").removeClass("inverted");
        Cookies.remove("inverted");
    }

    $("#act-invert").click(function(e) {
        var isInverted = Cookies.get("inverted") || false;

        $(":root").toggleClass("inverted");
        if (isInverted) {
            Cookies.remove("inverted");
        } else {
            Cookies.set("inverted", true);
        }
        e.preventDefault();
        e.stopPropagation();
    });
});