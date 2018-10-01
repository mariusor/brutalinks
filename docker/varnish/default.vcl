vcl 4.0;
import std;
backend default {
    .host = "application";
    .port = "3000";
}
sub vcl_recv {
    if (std.port(local.ip) == 80) {
        set req.http.x-redir = "https://" + req.http.host + req.url;
        return(synth(301));
    }
    if ((std.port(local.ip) == 6081) && (std.port(server.ip) == 443)) {
        set req.http.X-Forwarded-Proto = "https";
    }
    if (req.url ~ "^/assets/") {
        unset req.http.Cookie;
    }
}
sub vcl_backend_response {}
sub vcl_deliver {}
sub vcl_synth {
    if (resp.status == 301) {
        set resp.http.Location = req.http.x-redir;
        return (deliver);
    }
}
