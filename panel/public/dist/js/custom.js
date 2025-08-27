$(function () {
  "use strict";

  // Feather Icon Init Js
  feather.replace();

  $(".preloader").fadeOut();

  $(".left-sidebar").hover(
    function () {
      $(".navbar-header").addClass("expand-logo");
    },
    function () {
      $(".navbar-header").removeClass("expand-logo");
    }
  );
  // this is for close icon when navigation open in mobile view
  $(".nav-toggler").on("click", function () {
    $("#main-wrapper").toggleClass("show-sidebar");
    $(".nav-toggler i").toggleClass("ri-menu-2-line");
  });
  $(".nav-lock").on("click", function () {
    $("body").toggleClass("lock-nav");
    $(".nav-lock i").toggleClass("mdi-toggle-switch-off");
    $("body, .page-wrapper").trigger("resize");
  });
  $(".search-box a, .search-box .app-search .srh-btn").on("click", function () {
    $(".app-search").toggle(200);
    $(".app-search input").focus();
  });

  // ==============================================================
  // Right sidebar options
  // ==============================================================
  $(function () {
    $(".service-panel-toggle").on("click", function () {
      $(".customizer").toggleClass("show-service-panel");
    });
    $(".page-wrapper").on("click", function () {
      $(".customizer").removeClass("show-service-panel");
    });
  });

  $(function () {
    $(".nav-sidebar").on("click", function () {
      $(".nav-customizer").toggleClass("show-nav-sidebar");
    });
    $(".page-wrapper").on("click", function () {
      $(".nav-customizer").removeClass("show-nav-sidebar");
    });
  });

  // increase/decrease
  $(".increase").on("click", function () {
    var counter = $(".counter").val();
    counter++;
    $(".counter").val(counter);
  });

  $(".decrease").on("click", function () {
    var counter = $(".counter")
      .val()
      .replace(/[^0-9\.]/g, "");
    counter--;
    $(".counter").val(Math.abs(counter));
  });

  // ==============================================================
  // This is for the floating labels
  // ==============================================================
  $(".floating-labels .form-control")
    .on("focus blur", function (e) {
      $(this)
        .parents(".form-group")
        .toggleClass("focused", e.type === "focus" || this.value.length > 0);
    })
    .trigger("blur");

  // ==============================================================
  //tooltip
  // ==============================================================
  $(function () {
    var tooltipTriggerList = [].slice.call(
      document.querySelectorAll('[data-bs-toggle="tooltip"]')
    );
    var tooltipList = tooltipTriggerList.map(function (tooltipTriggerEl) {
      return new bootstrap.Tooltip(tooltipTriggerEl);
    });
  });
  // ==============================================================
  //Popover
  // ==============================================================
  $(function () {
    var popoverTriggerList = [].slice.call(
      document.querySelectorAll('[data-bs-toggle="popover"]')
    );
    var popoverList = popoverTriggerList.map(function (popoverTriggerEl) {
      return new bootstrap.Popover(popoverTriggerEl);
    });
  });

  // ==============================================================
  // Perfact scrollbar
  // ==============================================================
  $(".message-center, .customizer-body, .scrollable").perfectScrollbar({
    wheelPropagation: !0,
  });

  /*var ps = new PerfectScrollbar('.message-body');
    var ps = new PerfectScrollbar('.notifications');
    var ps = new PerfectScrollbar('.scroll-sidebar');
    var ps = new PerfectScrollbar('.customizer-body');*/

  // ==============================================================
  // Resize all elements
  // ==============================================================
  $("body, .page-wrapper").trigger("resize");
  $(".page-wrapper").delay(20).show();

  // ==============================================================
  // To do list
  // ==============================================================
  $(".list-task li label").click(function () {
    $(this).toggleClass("task-done");
  });

  // ==============================================================
  // Collapsable cards
  // ==============================================================
  $('a[data-action="collapse"]').on("click", function (e) {
    e.preventDefault();
    $(this)
      .closest(".card")
      .find('[data-action="collapse"] i')
      .toggleClass("ri-subtract-line ri-add-line");
    $(this).closest(".card").children(".card-body").collapse("toggle");
  });
  // Toggle fullscreen
  $('a[data-action="expand"]').on("click", function (e) {
    e.preventDefault();
    $(this)
      .closest(".card")
      .find('[data-action="expand"] i')
      .toggleClass("ri-fullscreen-line ri-fullscreen-exit-line");
    $(this).closest(".card").toggleClass("card-fullscreen");
  });
  // Close Card
  $('a[data-action="close"]').on("click", function () {
    $(this).closest(".card").removeClass().slideUp("fast");
  });
  // ==============================================================
  // LThis is for mega menu
  // ==============================================================
  $(".mega-dropdown").on("click", function (e) {
    e.stopPropagation();
  });
  // ==============================================================
  // Last month earning
  // ==============================================================
  $("#monthchart").sparkline([5, 6, 2, 9, 4, 7, 10, 12], {
    type: "bar",
    height: "35",
    barWidth: "4",
    resize: true,
    barSpacing: "4",
    barColor: "#1e88e5",
  });
  $("#lastmonthchart").sparkline([5, 6, 2, 9, 4, 7, 10, 12], {
    type: "bar",
    height: "35",
    barWidth: "4",
    resize: true,
    barSpacing: "4",
    barColor: "#7460ee",
  });
  var sparkResize;

  // ==============================================================
  // This is for the innerleft sidebar
  // ==============================================================
  $(".show-left-part").on("click", function () {
    $(".left-part").toggleClass("show-panel");
    $(".show-left-part").toggleClass("ri-menu-2-line");
  });

  // For Custom File Input
  $(".custom-file-input").on("change", function () {
    //get the file name
    var fileName = $(this).val();
    //replace the "Choose a file" label
    $(this).next(".custom-file-label").html(fileName);
  });
});
